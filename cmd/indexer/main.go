package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yourorg/code-reviewer/internal/github"
	"github.com/yourorg/code-reviewer/internal/indexer"
	"github.com/yourorg/code-reviewer/internal/knowledge"
)

func main() {
	// Subcommands
	indexCmd := flag.NewFlagSet("index", flag.ExitOnError)
	listCmd := flag.NewFlagSet("list", flag.ExitOnError)
	infoCmd := flag.NewFlagSet("info", flag.ExitOnError)

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "index":
		runIndex(indexCmd, os.Args[2:])
	case "list":
		runList(listCmd, os.Args[2:])
	case "info":
		runInfo(infoCmd, os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
	default:
		// Assume it's a repo URL for quick indexing
		runQuickIndex(os.Args[1:])
	}
}

func printUsage() {
	fmt.Println(`Framework Indexer - Index GitHub repositories for code review

Usage:
  indexer <command> [options]

Commands:
  index   Index a GitHub repository
  list    List indexed frameworks
  info    Show info about an indexed framework

Quick Usage:
  indexer <github-url>              Index a repo with auto-generated name
  indexer owner/repo                Index from GitHub shorthand

Examples:
  indexer index -name myframework -repo https://github.com/user/repo
  indexer index -name gin -repo gin-gonic/gin
  indexer list
  indexer info myframework
  indexer https://github.com/user/repo`)
}

func runIndex(fs *flag.FlagSet, args []string) {
	name := fs.String("name", "", "Name for the framework (required)")
	repoURL := fs.String("repo", "", "GitHub repository URL (required)")
	dataDir := fs.String("data-dir", "", "Directory to store indexed data")
	cacheDir := fs.String("cache-dir", "", "Directory to cache cloned repos")

	fs.Parse(args)

	if *repoURL == "" {
		if fs.NArg() > 0 {
			*repoURL = fs.Arg(0)
		} else {
			fmt.Fprintln(os.Stderr, "Error: -repo is required")
			fs.Usage()
			os.Exit(1)
		}
	}

	if *name == "" {
		// Auto-generate name from repo URL
		*name = extractName(*repoURL)
	}

	indexFramework(*name, *repoURL, *dataDir, *cacheDir)
}

func runQuickIndex(args []string) {
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	repoURL := args[0]
	name := extractName(repoURL)

	indexFramework(name, repoURL, "", "")
}

func runList(fs *flag.FlagSet, args []string) {
	dataDir := fs.String("data-dir", "", "Directory where frameworks are stored")
	fs.Parse(args)

	store := knowledge.NewStore(*dataDir)
	frameworks, err := store.ListFrameworks()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing frameworks: %v\n", err)
		os.Exit(1)
	}

	if len(frameworks) == 0 {
		fmt.Println("No frameworks indexed yet.")
		fmt.Println("Use 'indexer index -repo <github-url> -name <name>' to index a framework.")
		return
	}

	fmt.Println("Indexed Frameworks:")
	fmt.Println("-------------------")
	for _, f := range frameworks {
		fmt.Printf("  - %s\n", f)
	}
}

func runInfo(fs *flag.FlagSet, args []string) {
	dataDir := fs.String("data-dir", "", "Directory where frameworks are stored")
	fs.Parse(args)

	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "Error: framework name required")
		fmt.Fprintln(os.Stderr, "Usage: indexer info <framework-name>")
		os.Exit(1)
	}

	name := fs.Arg(0)
	store := knowledge.NewStore(*dataDir)

	framework, err := store.LoadFramework(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading framework: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Framework: %s\n", framework.Name)
	fmt.Printf("Repository: %s\n", framework.RepoURL)
	fmt.Printf("Local Path: %s\n", framework.LocalPath)
	fmt.Printf("Elements: %d\n", framework.ElementCount)
	fmt.Printf("Packages: %d\n", len(framework.Packages))
	fmt.Println("\nPackages:")
	for _, pkg := range framework.Packages {
		fmt.Printf("  - %s\n", pkg)
	}

	// Count by type
	typeCounts := make(map[string]int)
	for _, elem := range framework.Elements {
		typeCounts[elem.Type]++
	}
	fmt.Println("\nElements by Type:")
	for t, count := range typeCounts {
		fmt.Printf("  - %s: %d\n", t, count)
	}
}

func indexFramework(name, repoURL, dataDir, cacheDir string) {
	fmt.Printf("Indexing framework: %s\n", name)
	fmt.Printf("Repository: %s\n", repoURL)
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Clone/update repo
	repoClient := github.NewRepoClient(cacheDir)
	localPath, err := repoClient.CloneOrUpdate(ctx, repoURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error cloning repo: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Local path: %s\n\n", localPath)

	// Parse Go files
	fmt.Println("Parsing Go files...")
	parser := indexer.NewParser()
	elements, err := parser.ParseDirectory(localPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d code elements\n", len(elements))

	// Count by type
	typeCounts := make(map[string]int)
	for _, elem := range elements {
		typeCounts[elem.Type]++
	}
	for t, count := range typeCounts {
		fmt.Printf("  - %s: %d\n", t, count)
	}

	// Save to store
	fmt.Println("\nSaving to knowledge store...")
	store := knowledge.NewStore(dataDir)
	if err := store.SaveFramework(name, repoURL, localPath, elements); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nDone! Framework indexed successfully.")
	fmt.Printf("Use 'code-reviewer -framework %s <file>' to review code against this framework.\n", name)
}

func extractName(repoURL string) string {
	// Extract repo name from URL
	repoURL = filepath.Base(repoURL)
	if repoURL == "" {
		return "framework"
	}
	return repoURL
}
