import { ENVOY_HOST, getMetadata } from './transport'
import {
  GrpcWebImpl,
  UserRegistrationServiceClientImpl,
  GetUserRequest,
} from '../gen/userregistration/userregistration'

const client = new UserRegistrationServiceClientImpl(new GrpcWebImpl(ENVOY_HOST, {}))

export function getUser(username: string, jwt: string) {
  return client.GetUser(GetUserRequest.fromPartial({ username }), getMetadata(jwt))
}
