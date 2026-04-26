{{- define "document-service.fullname" -}}
{{- printf "%s-%s" .Release.Name "document-service" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "document-service.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/name: document-service
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "document-service.selectorLabels" -}}
app.kubernetes.io/name: document-service
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
