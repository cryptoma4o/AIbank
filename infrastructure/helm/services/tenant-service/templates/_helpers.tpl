{{- define "tenant-service.fullname" -}}
{{- printf "%s-%s" .Release.Name "tenant-service" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "tenant-service.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/name: tenant-service
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "tenant-service.selectorLabels" -}}
app.kubernetes.io/name: tenant-service
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
