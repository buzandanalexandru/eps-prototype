{{/*
eps.service renders the Deployment, Service and (optionally) Ingress of one Go binary.
Call from a sub-chart: {{ include "eps.service" (dict "name" "core" "ctx" .) }}
*/}}
{{- define "eps.labels" -}}
app.kubernetes.io/name: {{ .name }}
app.kubernetes.io/instance: {{ .ctx.Release.Name }}
app.kubernetes.io/part-of: eps
eps.dev/env: {{ .ctx.Values.global.env | quote }}
{{- end -}}

{{- define "eps.fullname" -}}
{{ printf "%s-%s" .ctx.Release.Name .name | trunc 63 | trimSuffix "-" }}
{{- end -}}

{{- define "eps.image" -}}
{{ printf "%s/eps-%s:%s" .ctx.Values.global.image.registry (.image | default .name) (.ctx.Values.global.image.tag | toString) }}
{{- end -}}

{{- define "eps.env" -}}
{{- $db := .ctx.Values.database | default dict }}
- name: PORT
  value: "8080"
- name: METRICS_PORT
  value: "9090"
{{- if ne ($db.enabled | toString) "false" }}
# The service's own login role (eps_core_login), never the branch owner; written by CI.
- name: DATABASE_URL
  valueFrom:
    secretKeyRef:
      name: {{ .ctx.Values.global.databaseSecret }}
      key: DATABASE_URL
      optional: true
- name: DB_MAX_CONNS
  value: {{ $db.maxConns | default 4 | quote }}
{{- end }}
- name: GOMAXPROCS
  valueFrom:
    resourceFieldRef:
      resource: limits.cpu
      divisor: "1"
- name: CORE_TOKEN_ISSUERS
  value: {{ .ctx.Values.global.tokenIssuers | default (printf "urn:eps:web:%s" .ctx.Values.global.env) | quote }}
{{- if not .ctx.Values.noRedis }}
# Redis requires a password (REDIS_PASSWORD, hex) and the core signs its cached copies with
# SNAPSHOT_CACHE_KEY: both from the runtime secret CI creates once per environment.
- name: REDIS_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ .ctx.Values.global.runtimeSecret }}
      key: REDIS_PASSWORD
      optional: true
- name: REDIS_URL
  value: {{ printf "redis://:$(REDIS_PASSWORD)@%s-redis:6379/0" .ctx.Release.Name | quote }}
- name: SNAPSHOT_CACHE_KEY
  valueFrom:
    secretKeyRef:
      name: {{ .ctx.Values.global.runtimeSecret }}
      key: SNAPSHOT_CACHE_KEY
      optional: true
{{- end }}
- name: CORS_ALLOWED_ORIGINS
  value: {{ .ctx.Values.global.corsAllowedOrigins | quote }}
- name: EPS_ENV
  value: {{ .ctx.Values.global.env | quote }}
{{- with .ctx.Values.paymentSecrets }}
# Step 4, only the keys this service needs: order links (ORDER_LINK_KEY), the core's key at the
# simulated providers (PSP_API_KEY), their knobs (PSP_ADMIN_TOKEN) and webhook keys
# (PSP_WEBHOOK_SECRET_A/B), from eps-runtime.
{{- range $k := . }}
- name: {{ $k }}
  valueFrom:
    secretKeyRef:
      name: {{ $.ctx.Values.global.runtimeSecret }}
      key: {{ $k }}
      # Required (review sec L6): CI creates every key before the release rolls out.
      optional: false
{{- end }}
{{- end }}
{{- with .ctx.Values.passKeys }}
# Step 5, the waiting room's Ed25519 pass keys from eps-runtime: the gateway signs
# (QUEUE_PASS_PRIVATE_KEY), the core only verifies (QUEUE_PASS_PUBLIC_KEY).
{{- range $k := . }}
- name: {{ $k }}
  valueFrom:
    secretKeyRef:
      name: {{ $.ctx.Values.global.runtimeSecret }}
      key: {{ $k }}
      optional: false
{{- end }}
{{- end }}
{{- with .ctx.Values.simKeys }}
# Step 6, the simulator's keys from eps-runtime: SIM_TOKEN_PRIVATE_KEY (the simulator only) signs
# simulated fans' tokens, SIM_TOKEN_PUBLIC_KEY (core, gateway) verifies them on rehearsals only,
# SIM_CONTROL_TOKEN guards the core's internal simulator API.
{{- range $k := . }}
- name: {{ $k }}
  valueFrom:
    secretKeyRef:
      name: {{ $.ctx.Values.global.runtimeSecret }}
      key: {{ $k }}
      optional: false
{{- end }}
{{- end }}
{{- with .ctx.Values.ticketKeys }}
# Step 7, the ticket signing key from eps-runtime (the core only; never Vercel or the browser).
{{- range $k := . }}
- name: {{ $k }}
  valueFrom:
    secretKeyRef:
      name: {{ $.ctx.Values.global.runtimeSecret }}
      key: {{ $k }}
      optional: false
{{- end }}
{{- end }}
{{- with .ctx.Values.extraEnv }}
{{- range $k, $v := . }}
- name: {{ $k }}
  value: {{ tpl (toString $v) $.ctx | quote }}
{{- end }}
{{- end }}
{{- with .ctx.Values.queueJoinsPerMinute }}
- name: QUEUE_JOINS_PER_MINUTE
  value: {{ . | quote }}
{{- end }}
{{- with .ctx.Values.queueNewGuestsPerMinute }}
- name: QUEUE_NEW_GUESTS_PER_MINUTE
  value: {{ . | quote }}
{{- end }}
{{- if .ctx.Values.paymentSecrets }}
- name: PSP_A_URL
  value: {{ printf "http://%s-psp-a" .ctx.Release.Name | quote }}
- name: PSP_B_URL
  value: {{ printf "http://%s-psp-b" .ctx.Release.Name | quote }}
- name: CORE_INTERNAL_URL
  value: {{ printf "http://%s-core" .ctx.Release.Name | quote }}
- name: RETURN_URL_ORIGINS
  value: {{ .ctx.Values.global.corsAllowedOrigins | quote }}
{{- with .ctx.Values.holdLimits }}
- name: HOLD_RATE_PER_MINUTE
  value: {{ .perMinute | quote }}
- name: HOLD_LIVE_PER_CLIENT
  value: {{ .live | quote }}
{{- end }}
{{- end }}
{{- with .ctx.Values.providerId }}
- name: PROVIDER_ID
  value: {{ . | quote }}
- name: PSP_PUBLIC_URL
  value: {{ printf "https://pay.%s.%s/%s" $.ctx.Values.global.env $.ctx.Values.global.baseDomain . | quote }}
{{- end }}
{{- end -}}

{{- define "eps.podSecurity" -}}
securityContext:
  runAsNonRoot: true
  runAsUser: 65532
  runAsGroup: 65532
  seccompProfile:
    type: RuntimeDefault
{{- end -}}

{{- define "eps.containerSecurity" -}}
securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop: ["ALL"]
{{- end -}}

{{- define "eps.service" -}}
{{- $v := .ctx.Values -}}
{{- if $v.enabled }}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "eps.fullname" . }}
  labels:
    {{- include "eps.labels" . | nindent 4 }}
spec:
  {{- if not (and $v.autoscaling $v.autoscaling.enabled) }}
  replicas: {{ $v.replicas | default 1 }}
  {{- end }}
  revisionHistoryLimit: 3
  strategy:
    type: RollingUpdate
    rollingUpdate: { maxUnavailable: 0, maxSurge: 1 }
  selector:
    matchLabels:
      app.kubernetes.io/name: {{ .name }}
      app.kubernetes.io/instance: {{ .ctx.Release.Name }}
  template:
    metadata:
      labels:
        {{- include "eps.labels" . | nindent 8 }}
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9090"
        prometheus.io/path: /metrics
        # Step 6: the on-sale services are scraped every 2 s, the others every 15 s.
        eps.dev/scrape-interval: {{ ($v.scrapeInterval | default "15s") | quote }}
        {{- with .ctx.Values.global.rolloutId }}
        # Restarts the pods when CI rewrites the database secret (new branch or new password).
        eps.dev/rollout: {{ . | quote }}
        {{- end }}
    spec:
      {{- if $v.serviceAccount }}
      # Step 6: the simulator's own account, allowed only to update the cluster's rehearsal Lease.
      serviceAccountName: {{ include "eps.fullname" . }}
      automountServiceAccountToken: true
      {{- else }}
      automountServiceAccountToken: false
      {{- end }}
      terminationGracePeriodSeconds: 30
      {{- include "eps.podSecurity" . | nindent 6 }}
      containers:
        - name: {{ .name }}
          image: {{ include "eps.image" . }}
          imagePullPolicy: {{ .ctx.Values.global.image.pullPolicy }}
          ports:
            - name: http
              containerPort: 8080
            - name: metrics
              containerPort: 9090
            {{- with $v.internalPort }}
            # Step 6: the core's simulator API (in the cluster only; no ingress routes to it).
            - name: internal
              containerPort: {{ . }}
            {{- end }}
          env:
            {{- include "eps.env" . | nindent 12 }}
          # Readiness turns false on SIGTERM; the sleep lets endpoints and the ingress drop the pod
          # before the process stops accepting (distroless has no shell, hence the sleep action).
          lifecycle:
            preStop:
              sleep: { seconds: 5 }
          livenessProbe:
            httpGet: { path: /healthz, port: http }
            periodSeconds: 10
            timeoutSeconds: 3
            failureThreshold: 3
          readinessProbe:
            httpGet: { path: /readyz, port: http }
            periodSeconds: 5
            timeoutSeconds: 3
            failureThreshold: 3
          startupProbe:
            httpGet: { path: /healthz, port: http }
            periodSeconds: 2
            timeoutSeconds: 3
            failureThreshold: 30
          resources:
            {{- toYaml $v.resources | nindent 12 }}
          {{- include "eps.containerSecurity" . | nindent 10 }}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ include "eps.fullname" . }}
  labels:
    {{- include "eps.labels" . | nindent 4 }}
spec:
  selector:
    app.kubernetes.io/name: {{ .name }}
    app.kubernetes.io/instance: {{ .ctx.Release.Name }}
  ports:
    - name: http
      port: 80
      targetPort: http
    {{- with $v.internalPort }}
    - name: internal
      port: {{ . }}
      targetPort: internal
    {{- end }}
{{- if not (and $v.pdb (eq ($v.pdb.enabled | toString) "false")) }}
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ include "eps.fullname" . }}
  labels:
    {{- include "eps.labels" . | nindent 4 }}
spec:
  # maxUnavailable (not minAvailable) so a single-replica preview never blocks a node drain.
  maxUnavailable: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: {{ .name }}
      app.kubernetes.io/instance: {{ .ctx.Release.Name }}
{{- end }}
{{- if and $v.autoscaling $v.autoscaling.enabled }}
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: {{ include "eps.fullname" . }}
  labels:
    {{- include "eps.labels" . | nindent 4 }}
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: {{ include "eps.fullname" . }}
  minReplicas: {{ $v.autoscaling.minReplicas | default 2 }}
  maxReplicas: {{ $v.autoscaling.maxReplicas | default 10 }}
  metrics:
    - type: Resource
      resource:
        name: cpu
        target: { type: Utilization, averageUtilization: {{ $v.autoscaling.targetCPU | default 70 }} }
{{- end }}
{{- if and $v.ingress $v.ingress.enabled }}
{{- $host := printf "%s.%s.%s" $v.ingress.host .ctx.Values.global.env .ctx.Values.global.baseDomain }}
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{ include "eps.fullname" . }}
  labels:
    {{- include "eps.labels" . | nindent 4 }}
  annotations:
    cert-manager.io/cluster-issuer: {{ .ctx.Values.global.ingress.clusterIssuer }}
spec:
  ingressClassName: {{ .ctx.Values.global.ingress.className }}
  tls:
    - hosts: [{{ $host | quote }}]
      secretName: {{ printf "%s-%s-tls" .name .ctx.Values.global.env | trunc 63 | trimSuffix "-" }}
  rules:
    - host: {{ $host | quote }}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: {{ include "eps.fullname" . }}
                port:
                  name: http
{{- if and $v.ingress.publicRateLimit $v.ingress.publicRateLimit.enabled }}
---
# The public endpoints (/v1/public/...) behind a rate limit (security review): shops reach them
# through the web app's CDN-cached route, so direct traffic is small; a flood is cut at the edge.
# Behind the load balancer every request shares one source, so this limit is per core host.
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: {{ include "eps.fullname" . }}-public-ratelimit
  labels:
    {{- include "eps.labels" . | nindent 4 }}
spec:
  rateLimit:
    average: {{ $v.ingress.publicRateLimit.average | default 200 }}
    burst: {{ $v.ingress.publicRateLimit.burst | default 400 }}
    period: 1s
    sourceCriterion:
      requestHost: true
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{ include "eps.fullname" . }}-public
  labels:
    {{- include "eps.labels" . | nindent 4 }}
  annotations:
    traefik.ingress.kubernetes.io/router.middlewares: {{ printf "%s-%s-public-ratelimit@kubernetescrd" .ctx.Release.Namespace (include "eps.fullname" .) | quote }}
spec:
  ingressClassName: {{ .ctx.Values.global.ingress.className }}
  tls:
    - hosts: [{{ $host | quote }}]
      secretName: {{ printf "%s-%s-tls" .name .ctx.Values.global.env | trunc 63 | trimSuffix "-" }}
  rules:
    - host: {{ $host | quote }}
      http:
        paths:
          - path: /v1/public/
            pathType: Prefix
            backend:
              service:
                name: {{ include "eps.fullname" . }}
                port:
                  name: http
{{- end }}
{{- end }}
{{- end }}
{{- end -}}
