{{/*
Expand the name of the chart.
*/}}
{{- define "code-reviewer.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
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
Create the name of the configmap
*/}}
{{- define "code-reviewer.configmapName" -}}
{{- include "code-reviewer.fullname" . }}-config
{{- end }}

{{/*
Create the name of the secret
*/}}
{{- define "code-reviewer.secretName" -}}
{{- include "code-reviewer.fullname" . }}-secret
{{- end }}

{{/*
Create the name of the PVC
*/}}
{{- define "code-reviewer.pvcName" -}}
{{- include "code-reviewer.fullname" . }}-data
{{- end }}
