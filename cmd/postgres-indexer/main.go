// Command postgres-indexer ingests PostgreSQL knowledge into the RAG system.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/yourorg/code-reviewer/internal/postgres"
	"github.com/yourorg/code-reviewer/internal/rag"
)

func main() {
	// Subcommands
	ingestCmd := flag.NewFlagSet("ingest", flag.ExitOnError)
	listCmd := flag.NewFlagSet("list", flag.ExitOnError)
	searchCmd := flag.NewFlagSet("search", flag.ExitOnError)
	statsCmd := flag.NewFlagSet("stats", flag.ExitOnError)

	// Common flags
	llmURL := getEnv("LLM_URL", "http://localhost:11434")
	embedModel := getEnv("EMBED_MODEL", "nomic-embed-text")
	embedDim := 768
	knowledgeDir := getEnv("KNOWLEDGE_DIR", "")

	if knowledgeDir == "" {
		homeDir, _ := os.UserHomeDir()
		knowledgeDir = filepath.Join(homeDir, ".code-reviewer", "postgres-knowledge")
	}

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "ingest":
		// Ingest flags
		all := ingestCmd.Bool("all", false, "Ingest all default sources")
		repo := ingestCmd.String("repo", "", "GitHub repository URL to ingest")
		name := ingestCmd.String("name", "", "Name for the source")
		dir := ingestCmd.String("dir", "", "Local directory to ingest")
		orgStandards := ingestCmd.String("org-standards", "", "Path to organization standards")
		workDir := ingestCmd.String("work-dir", "", "Working directory for cloning repos")

		ingestCmd.Parse(os.Args[2:])

		if *workDir == "" {
			*workDir = filepath.Join(knowledgeDir, "repos")
		}

		// Create components
		embedder := rag.NewOllamaEmbedder(llmURL, embedModel, embedDim)
		vectorStore := rag.NewMemoryVectorStore(knowledgeDir)
		ingester := postgres.NewIngester(embedder, vectorStore, *workDir)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		if *all {
			log.Println("Ingesting all default PostgreSQL sources...")
			if err := ingester.IngestAll(ctx); err != nil {
				log.Fatalf("Ingestion failed: %v", err)
			}
			log.Println("Ingestion complete!")
		} else if *repo != "" {
			if *name == "" {
				log.Fatal("-name is required when ingesting a repository")
			}
			log.Printf("Ingesting repository: %s as %s", *repo, *name)
			source := postgres.IngestSource{
				Name:     *name,
				RepoURL:  *repo,
				Branch:   "main",
				PathGlob: "**/*.md",
			}
			if err := ingester.IngestSource(ctx, source); err != nil {
				log.Fatalf("Ingestion failed: %v", err)
			}
			log.Println("Ingestion complete!")
		} else if *dir != "" {
			if *name == "" {
				*name = filepath.Base(*dir)
			}
			log.Printf("Ingesting directory: %s as %s", *dir, *name)
			if err := ingester.IngestDirectory(ctx, *name, *dir); err != nil {
				log.Fatalf("Ingestion failed: %v", err)
			}
			log.Println("Ingestion complete!")
		} else if *orgStandards != "" {
			log.Printf("Ingesting organization standards from: %s", *orgStandards)
			if err := ingester.IngestOrgStandards(ctx, "custom", *orgStandards); err != nil {
				log.Fatalf("Ingestion failed: %v", err)
			}
			log.Println("Ingestion complete!")
		} else {
			fmt.Println("Usage: postgres-indexer ingest [-all] [-repo URL -name NAME] [-dir PATH] [-org-standards PATH]")
			os.Exit(1)
		}

	case "list":
		listCmd.Parse(os.Args[2:])

		vectorStore := rag.NewMemoryVectorStore(knowledgeDir)
		ctx := context.Background()

		collections, err := vectorStore.ListCollections(ctx)
		if err != nil {
			log.Fatalf("Failed to list collections: %v", err)
		}

		fmt.Println("Collections:")
		for _, name := range collections {
			size, _ := vectorStore.CollectionSize(ctx, name)
			fmt.Printf("  - %s (%d documents)\n", name, size)
		}

	case "search":
		query := searchCmd.String("query", "", "Search query")
		category := searchCmd.String("category", "", "Category to search (performance, security, standards, migrations)")
		topK := searchCmd.Int("top-k", 5, "Number of results to return")

		searchCmd.Parse(os.Args[2:])

		if *query == "" {
			fmt.Println("Usage: postgres-indexer search -query \"your search query\" [-category NAME] [-top-k N]")
			os.Exit(1)
		}

		embedder := rag.NewOllamaEmbedder(llmURL, embedModel, embedDim)
		vectorStore := rag.NewMemoryVectorStore(knowledgeDir)
		retriever := rag.NewRetriever(embedder, vectorStore, nil)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var collections []string
		if *category != "" {
			collections = []string{"postgres_" + *category}
		} else {
			collections = []string{"postgres_performance", "postgres_security", "postgres_standards", "postgres_migrations"}
		}

		resp, err := retriever.Retrieve(ctx, rag.RetrievalRequest{
			Query:       *query,
			Collections: collections,
			TopK:        *topK,
			MinScore:    0.5,
		})
		if err != nil {
			log.Fatalf("Search failed: %v", err)
		}

		fmt.Printf("\nSearch results for: %s\n", *query)
		fmt.Println(resp.FormattedContext)

	case "stats":
		statsCmd.Parse(os.Args[2:])

		vectorStore := rag.NewMemoryVectorStore(knowledgeDir)
		ctx := context.Background()

		collections, err := vectorStore.ListCollections(ctx)
		if err != nil {
			log.Fatalf("Failed to get stats: %v", err)
		}

		fmt.Println("\nKnowledge Base Statistics")
		fmt.Println("=" + string(make([]byte, 40)))

		totalDocs := 0
		for _, name := range collections {
			size, _ := vectorStore.CollectionSize(ctx, name)
			totalDocs += size
			fmt.Printf("  %-25s %5d documents\n", name+":", size)
		}
		fmt.Println("  " + string(make([]byte, 40)))
		fmt.Printf("  %-25s %5d documents\n", "Total:", totalDocs)

	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`PostgreSQL Knowledge Indexer

Usage:
  postgres-indexer <command> [options]

Commands:
  ingest    Ingest knowledge documents into the RAG system
  list      List all collections in the knowledge base
  search    Search the knowledge base
  stats     Show knowledge base statistics

Examples:
  # Ingest all default postgres-ai sources
  postgres-indexer ingest -all

  # Ingest a specific repository
  postgres-indexer ingest -repo https://github.com/postgres-ai/postgres-howtos.git -name postgres-howtos

  # Ingest a local directory
  postgres-indexer ingest -dir ./my-pg-docs -name my-docs

  # Ingest organization standards
  postgres-indexer ingest -org-standards ./standards/postgresql

  # Search the knowledge base
  postgres-indexer search -query "how to create indexes concurrently"

  # Search in a specific category
  postgres-indexer search -query "SQL injection prevention" -category security

  # List collections
  postgres-indexer list

  # Show statistics
  postgres-indexer stats

Environment Variables:
  LLM_URL        LLM server URL (default: http://localhost:11434)
  EMBED_MODEL    Embedding model name (default: nomic-embed-text)
  KNOWLEDGE_DIR  Knowledge base storage directory`)
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
