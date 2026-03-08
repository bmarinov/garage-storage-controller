{{/*
Chart name
*/}}
{{- define "garage-controller.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "garage-controller.fullname" -}}
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
{{- define "garage-controller.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "garage-controller.labels" -}}
helm.sh/chart: {{ include "garage-controller.chart" . }}
{{ include "garage-controller.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "garage-controller.selectorLabels" -}}
app.kubernetes.io/name: {{ include "garage-controller.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Service Account and Roles
*/}}
{{- define "garage-controller.serviceAccountName" -}}
{{- default (include "garage-controller.fullname" .) .Values.serviceAccount.name }}
{{- end }}

{{- define "garage-controller.leaderElectionRoleName" -}}
{{- printf "%s-leader-election" (include "garage-controller.fullname" .) }}
{{- end }}

{{- define "garage-controller.clusterRoleName" -}}
{{- include "garage-controller.fullname" . }}
{{- end }}
