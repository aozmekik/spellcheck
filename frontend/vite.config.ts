import { defineConfig } from 'vite'

// The dev server forwards API calls to the Go service, so the browser talks to
// one origin, as it would behind a reverse proxy, and the API needs no CORS.
export default defineConfig({
  server: {
    proxy: { '/v1': 'http://127.0.0.1:8080' },
  },
})
