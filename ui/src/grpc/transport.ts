import { grpc } from '@improbable-eng/grpc-web'

// In development, point at Envoy on localhost:8080.
// In production (K8s), set VITE_ENVOY_HOST to the ingress URL.
export const ENVOY_HOST = import.meta.env.VITE_ENVOY_HOST ?? 'http://localhost:8080'

export function getMetadata(jwt?: string): grpc.Metadata {
  const md = new grpc.Metadata()
  if (jwt) {
    md.set('authorization', `Bearer ${jwt}`)
  }
  return md
}
