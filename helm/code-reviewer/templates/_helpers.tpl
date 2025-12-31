{{/*
Expand the name of the chart.
*/}}
{{- define "code-reviewer.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "code-reviewer.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "code-reviewer.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "code-reviewer.labels" -}}
helm.sh/chart: {{ include "code-reviewer.chart" . }}
{{ include "code-reviewer.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "code-reviewer.selectorLabels" -}}
app.kubernetes.io/name: {{ include "code-reviewer.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Server selector labels
*/}}
{{- define "code-reviewer.serverSelectorLabels" -}}
{{ include "code-reviewer.selectorLabels" . }}
app.kubernetes.io/component: server
{{- end }}

{{/*
Worker selector labels
*/}}
{{- define "code-reviewer.workerSelectorLabels" -}}
{{ include "code-reviewer.selectorLabels" . }}
app.kubernetes.io/component: worker
{{- end }}

{{/*
Ollama selector labels
*/}}
{{- define "code-reviewer.ollamaSelectorLabels" -}}
{{ include "code-reviewer.selectorLabels" . }}
app.kubernetes.io/component: ollama
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "code-reviewer.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "code-reviewer.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Ollama URL
*/}}
{{- define "code-reviewer.ollamaUrl" -}}
{{- if .Values.ollama.enabled }}
{{- printf "http://%s-ollama:11434" (include "code-reviewer.fullname" .) }}
{{- else }}
{{- .Values.ollama.externalUrl }}
{{- end }}
{{- end }}

{{/*
Temporal host
*/}}
{{- define "code-reviewer.temporalHost" -}}
{{- if .Values.temporal.enabled }}
{{- printf "%s-temporal-frontend:7233" .Release.Name }}
{{- else }}
{{- .Values.temporal.externalHost }}
{{- end }}
{{- end }}

{{/*
GitLab secret name
*/}}
{{- define "code-reviewer.gitlabSecretName" -}}
{{- if .Values.gitlab.existingSecret }}
{{- .Values.gitlab.existingSecret }}
{{- else }}
{{- printf "%s-gitlab" (include "code-reviewer.fullname" .) }}
{{- end }}
{{- end }}
