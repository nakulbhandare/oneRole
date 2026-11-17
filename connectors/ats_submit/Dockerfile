FROM node:22-bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    chromium \
    fonts-liberation \
    fonts-noto-color-emoji \
    tini \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

ENV PUPPETEER_SKIP_CHROMIUM_DOWNLOAD=true \
    PUPPETEER_EXECUTABLE_PATH=/usr/bin/chromium \
    CHROME_PATH=/usr/bin/chromium \
    NODE_ENV=production \
    PORT=3001

COPY connectors/ats_submit/package*.json ./
RUN npm install --omit=dev

COPY connectors/ats_submit/fast_submitter.js ./fast_submitter.js
COPY connectors/ats_submit/browser_submitter.js ./browser_submitter.js
COPY connectors/ats_submit/worker_daemon.js ./worker_daemon.js
COPY oneRole_UI/public/Nakul_Bhandare_Resume.pdf ./oneRole_UI/public/Nakul_Bhandare_Resume.pdf
COPY oneRole_UI/public/Nakul_Bhandare_Resume.pdf ./Nakul_Bhandare_Resume.pdf

EXPOSE 3001

ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["node", "worker_daemon.js"]
