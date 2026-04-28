{{/*
Expand the name of the chart. Honors nameOverride and the alias used
in dependency declarations (e.g. tenant-service alias on aibank-service).
*/}}
{{- define "aibank-service.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully-qualified app name. Truncated to 63 chars
because some Kubernetes name fields are limited to that.
*/}}
{{- define "aibank-service.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Chart label string used for app.kubernetes.io/version selectors.
*/}}
{{- define "aibank-service.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels applied to every rendered resource.
*/}}
{{- define "aibank-service.labels" -}}
helm.sh/chart: {{ include "aibank-service.chart" . }}
{{ include "aibank-service.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: aibank
{{- end -}}

{{/*
Selector labels — must remain stable across upgrades because they go
into Deployment.spec.selector which is immutable.
*/}}
{{- define "aibank-service.selectorLabels" -}}
app.kubernetes.io/name: {{ include "aibank-service.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
ServiceAccount name resolution.
*/}}
{{- define "aibank-service.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "aibank-service.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
Resolve the container image reference. Prefer digest when set,
fall back to tag, and to .Chart.AppVersion when tag is empty.
*/}}
{{- define "aibank-service.image" -}}
{{- $repo := .Values.image.repository -}}
{{- if .Values.image.digest -}}
{{- printf "%s@%s" $repo .Values.image.digest -}}
{{- else -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag -}}
{{- printf "%s:%s" $repo $tag -}}
{{- end -}}
{{- end -}}

{{/*
Resolve the named port for probes, falling back to service.portName.
*/}}
{{- define "aibank-service.probePort" -}}
{{- if . -}}
{{- . -}}
{{- else -}}
http
{{- end -}}
{{- end -}}
