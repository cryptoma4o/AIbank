{{- define "notification-service.fullname" -}}
{{- printf "%s-%s" .Release.Name "notification-service" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "notification-service.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/name: notification-service
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "notification-service.selectorLabels" -}}
app.kubernetes.io/name: notification-service
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
