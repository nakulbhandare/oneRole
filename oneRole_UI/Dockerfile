# ============================================================================
# OneRole Enterprise UI Microservice — Production Dockerfile
# ============================================================================
FROM node:20-alpine

WORKDIR /app

RUN apk add --no-cache wget curl && \
    addgroup -g 1001 -S onerole && \
    adduser -u 1001 -S onerole -G onerole

COPY package*.json ./
RUN npm install --omit=dev

COPY server.js ./
COPY public/ ./public/

USER onerole

ENV PORT=8082
ENV NODE_ENV=production
ENV BACKEND_URL=http://onerole-api:8088
ENV NATS_MONITOR_URL=http://nats:8222

EXPOSE 8082

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://localhost:8082/healthz || exit 1

CMD ["node", "server.js"]
