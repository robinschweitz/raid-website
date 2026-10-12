const upstream = 'https://db.rising-gods.de/'

addEventListener('fetch', (event) => event.respondWith(handleRequest(event.request)))

async function handleRequest(request) {
  if (request.method !== 'GET') return new Response('Method not allowed', { status: 405 })
  if (request.headers.get('X-Rising-Gods-Proxy-Token') !== UPSTREAM_AUTH_TOKEN) {
    return new Response('Unauthorized', { status: 401 })
  }

  const url = new URL(request.url)
  if (url.pathname !== '/') return new Response('Not found', { status: 404 })
  const response = await fetch(upstream + url.search, {
    headers: { 'User-Agent': 'Bratwurst-des-Schicksals/1.0' },
    cf: { cacheTtl: 300, cacheEverything: true },
  })
  return new Response(response.body, {
    status: response.status,
    headers: { 'Content-Type': response.headers.get('Content-Type') ?? 'text/html; charset=utf-8' },
  })
}
