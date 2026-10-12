const upstream = 'https://db.rising-gods.de/'

addEventListener('fetch', (event) => event.respondWith(handleRequest(event.request)))

function diagnosticHeaders(requestID, upstreamStatus) {
  const headers = new Headers({
    'X-Rising-Gods-Proxy-Request-ID': requestID,
    'X-Rising-Gods-Proxy': 'rising-gods-proxy',
  })
  if (upstreamStatus !== '') headers.set('X-Rising-Gods-Proxy-Upstream-Status', upstreamStatus)
  return headers
}

async function handleRequest(request) {
  if (request.method !== 'GET') return new Response('Method not allowed', { status: 405 })
  if (request.headers.get('X-Rising-Gods-Proxy-Token') !== UPSTREAM_AUTH_TOKEN) {
    return new Response('Unauthorized', { status: 401 })
  }

  const url = new URL(request.url)
  if (url.pathname !== '/') return new Response('Not found', { status: 404 })
  const requestID = crypto.randomUUID()
  try {
    const response = await fetch(upstream + url.search, {
      headers: { 'User-Agent': 'Bratwurst-des-Schicksals/1.0' },
    })
    const headers = diagnosticHeaders(requestID, String(response.status))
    headers.set('Content-Type', response.headers.get('Content-Type') ?? 'text/html; charset=utf-8')
    if (!response.ok) console.log(JSON.stringify({ requestID, upstreamStatus: response.status }))
    return new Response(response.body, { status: response.status, headers })
  } catch (error) {
    console.log(JSON.stringify({ requestID, upstreamError: String(error) }))
    return new Response('Rising-Gods upstream request failed', {
      status: 502,
      headers: diagnosticHeaders(requestID, 'fetch_error'),
    })
  }
}
