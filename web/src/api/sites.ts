import { api } from './client'

// Site management API - matches Go backend /api/sites endpoints
export const sitesAPI = {
  // List all sites
  list: () => api.get('/sites').then((r) => r.data),

  // Get single site with decrypted fields
  get: (id: string | number) => api.get(`/sites/${id}`).then((r) => r.data),

  // Create a new site
  create: (data: Record<string, unknown>) =>
    api.post('/sites', data).then((r) => r.data),

  // Update existing site
  update: (id: string | number, data: Record<string, unknown>) =>
    api.put(`/sites/${id}`, data).then((r) => r.data),

  // Delete a site
  remove: (id: string | number) =>
    api.delete(`/sites/${id}`).then((r) => r.data),

  // Test site connectivity
  test: (id: string | number) =>
    api.post(`/sites/${id}/test`).then((r) => r.data),

  // Get supported site types
  types: () => api.get('/sites/types').then((r) => r.data),

  // Get supported auth types
  authTypes: () => api.get('/sites/auth-types').then((r) => r.data),
}
