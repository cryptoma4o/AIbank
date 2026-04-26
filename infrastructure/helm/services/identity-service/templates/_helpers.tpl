{{- define "identity-service.fullname" -}}
{{- printf "%s-%s" .Release.Name "identity-service" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "identity-service.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/name: identity-service
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "identity-service.selectorLabels" -}}
app.kubernetes.io/name: identity-service
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
