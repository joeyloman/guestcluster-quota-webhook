{{/*
Expand the name of the chart.
*/}}
{{- define "guestcluster-quota-webhook.name" -}}
{{- default .Chart.Name .Values.guestclusterquotawebhook.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "guestcluster-quota-webhook.fullname" -}}
{{- if .Values.guestclusterquotawebhook.fullnameOverride }}
{{- .Values.guestclusterquotawebhook.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.guestclusterquotawebhook.nameOverride }}
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
{{- define "guestcluster-quota-webhook.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "guestcluster-quota-webhook.labels" -}}
helm.sh/chart: {{ include "guestcluster-quota-webhook.chart" . }}
{{ include "guestcluster-quota-webhook.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "guestcluster-quota-webhook.selectorLabels" -}}
app.kubernetes.io/name: {{ include "guestcluster-quota-webhook.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "guestcluster-quota-webhook.serviceAccountName" -}}
{{- if .Values.guestclusterquotawebhook.serviceAccount.create }}
{{- default (include "guestcluster-quota-webhook.fullname" .) .Values.guestclusterquotawebhook.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.guestclusterquotawebhook.serviceAccount.name }}
{{- end }}
{{- end }}
