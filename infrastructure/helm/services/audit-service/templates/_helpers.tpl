{{- define "audit-service.fullname" -}}
{{- printf "%s-%s" .Release.Name "audit-service" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "audit-service.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/name: audit-service
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "audit-service.selectorLabels" -}}
app.kubernetes.io/name: audit-service
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
