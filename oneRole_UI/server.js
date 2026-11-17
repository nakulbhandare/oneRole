const http = require('http');
const fs = require('fs');
const path = require('path');
const { URL } = require('url');

const PORT = parseInt(process.env.PORT || '8082', 10);
const BACKEND_URL = process.env.BACKEND_URL || 'http://localhost:8088';
const NATS_MONITOR_URL = process.env.NATS_MONITOR_URL || 'http://localhost:8222';
const PUBLIC_DIR = path.join(__dirname, 'public');

const MIME_TYPES = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.svg': 'image/svg+xml',
  '.ico': 'image/x-icon',
  '.pdf': 'application/pdf'
};

function proxyRequest(req, res, targetBase) {
  const targetUrl = new URL(req.url, targetBase);
  const isHttps = targetUrl.protocol === 'https:';
  const client = isHttps ? require('https') : require('http');

  const headers = { ...req.headers };
  headers.host = targetUrl.host;
  delete headers['connection'];
  delete headers['origin'];
  delete headers['referer'];

  const options = {
    hostname: targetUrl.hostname,
    port: targetUrl.port || (isHttps ? 443 : 80),
    path: targetUrl.pathname + targetUrl.search,
    method: req.method,
    headers: headers,
    timeout: 180000
  };

  const proxyReq = client.request(options, (proxyRes) => {
    const resHeaders = { ...proxyRes.headers };
    resHeaders['access-control-allow-origin'] = '*';
    resHeaders['access-control-allow-methods'] = 'GET, POST, PUT, PATCH, DELETE, OPTIONS';
    resHeaders['access-control-allow-headers'] = 'Authorization, Content-Type, Idempotency-Key, X-Tenant-ID, X-CSRF-Token, X-Request-ID';
    res.writeHead(proxyRes.statusCode, resHeaders);
    proxyRes.pipe(res, { end: true });
  });

  proxyReq.on('error', (err) => {
    console.error(`[Proxy Error] ${req.method} ${req.url} -> ${targetBase}:`, err.message);
    if (!res.headersSent) {
      res.writeHead(502, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({
        error: 'Proxy Error: Unable to reach backend service',
        details: err.message,
        target: targetBase
      }));
    }
  });

  proxyReq.on('timeout', () => {
    proxyReq.destroy();
    if (!res.headersSent) {
      res.writeHead(504, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'Backend gateway timeout' }));
    }
  });

  req.pipe(proxyReq, { end: true });
}

const server = http.createServer((req, res) => {
  const reqStart = Date.now();

  // Global CORS headers for seamless integration
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, PATCH, DELETE, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Authorization, Content-Type, Idempotency-Key, X-Tenant-ID, X-CSRF-Token, X-Request-ID');

  if (req.method === 'OPTIONS') {
    res.writeHead(204);
    res.end();
    return;
  }

  // 1. Proxy API endpoints to Go backend
  if (req.url.startsWith('/api/') || req.url === '/docs' || req.url === '/swagger' || req.url.startsWith('/swagger/')) {
    proxyRequest(req, res, BACKEND_URL);
    return;
  }

  // 2. Telemetry proxy for NATS JetStream stats
  if (req.url.startsWith('/telemetry/nats')) {
    const natsUrl = `${NATS_MONITOR_URL}/jsz?streams=true&consumers=true`;
    http.get(natsUrl, (natsRes) => {
      let data = '';
      natsRes.on('data', chunk => data += chunk);
      natsRes.on('end', () => {
        res.writeHead(natsRes.statusCode || 200, { 'Content-Type': 'application/json' });
        res.end(data);
      });
    }).on('error', (err) => {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({
        streams: 4,
        messages: 703,
        consumers: 4,
        mock_fallback: true,
        error: err.message
      }));
    });
    return;
  }

  // 3. Health check for oneRole_UI microservice
  if (req.url === '/healthz' || req.url === '/health') {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({
      status: 'ok',
      service: 'oneRole_UI',
      port: PORT,
      backend: BACKEND_URL,
      uptime: process.uptime()
    }));
    return;
  }

  // 4. Static file serving from public/
  let parsedUrl;
  try {
    parsedUrl = new URL(req.url, `http://localhost:${PORT}`);
  } catch (e) {
    res.writeHead(400, { 'Content-Type': 'text/plain' });
    res.end('Bad Request');
    return;
  }

  let filePath = path.join(PUBLIC_DIR, parsedUrl.pathname === '/' ? 'index.html' : parsedUrl.pathname);

  // Security check: prevent directory traversal
  if (!filePath.startsWith(PUBLIC_DIR)) {
    res.writeHead(403, { 'Content-Type': 'text/plain' });
    res.end('Forbidden');
    return;
  }

  fs.stat(filePath, (err, stats) => {
    if (err || !stats.isFile()) {
      const ext = path.extname(parsedUrl.pathname);
      if (ext) {
        // If a file extension was requested (e.g. /browse/styles.css), try resolving by basename in PUBLIC_DIR
        const basenameFile = path.join(PUBLIC_DIR, path.basename(parsedUrl.pathname));
        if (fs.existsSync(basenameFile) && fs.statSync(basenameFile).isFile()) {
          filePath = basenameFile;
        } else {
          res.writeHead(404, { 'Content-Type': 'text/plain' });
          console.log("404 Not Found: " + filePath); res.end('404 Not Found');
          return;
        }
      } else {
        // Fallback to index.html for clean SPA routes (/dashboard, /browse, /auto-apply, etc.)
        filePath = path.join(PUBLIC_DIR, 'index.html');
      }
    }

    fs.readFile(filePath, (readErr, content) => {
      if (readErr) {
        res.writeHead(404, { 'Content-Type': 'text/plain' });
        console.log("404 Not Found: " + filePath); res.end('404 Not Found');
        return;
      }
      const ext = path.extname(filePath).toLowerCase();
      const mime = MIME_TYPES[ext] || 'application/octet-stream';
      res.writeHead(200, {
        'Content-Type': mime,
        'Cache-Control': 'no-cache, no-store, must-revalidate'
      });
      res.end(content);
    });
  });
});

server.listen(PORT, () => {
  console.log(`🚀 [oneRole_UI] Microservice running on port ${PORT}`); console.log("Serving from: " + PUBLIC_DIR);
  console.log(`🔗 [oneRole_UI] Backend Proxy -> ${BACKEND_URL}`);
  console.log(`📊 [oneRole_UI] NATS Telemetry Proxy -> ${NATS_MONITOR_URL}`);
});

process.on('SIGINT', () => {
  console.log('\n[oneRole_UI] Shutting down gracefully...');
  server.close(() => process.exit(0));
});

process.on('SIGTERM', () => {
  console.log('\n[oneRole_UI] Received SIGTERM...');
  server.close(() => process.exit(0));
});
