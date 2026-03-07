{{- define "vector-kv.name" -}}
vector-kv
{{- end -}}

{{- define "vector-kv.labels" -}}
app.kubernetes.io/name: {{ include "vector-kv.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "vector-kv.selectorLabels" -}}
app.kubernetes.io/name: {{ include "vector-kv.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
