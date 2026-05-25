import { ENVOY_HOST, getMetadata } from './transport'
import {
  GrpcWebImpl,
  UserRegistrationServiceClientImpl,
  GetUserRequest,
  CompleteRegistrationRequest,
} from '../gen/userregistration/userregistration'

const client = new UserRegistrationServiceClientImpl(new GrpcWebImpl(ENVOY_HOST, {}))

export function getUser(username: string, jwt: string) {
  return client.GetUser(GetUserRequest.fromPartial({ username }), getMetadata(jwt))
}

export function completeRegistration(cognitoSub: string, username: string, email: string, jwt: string) {
  return client.CompleteRegistration(
    CompleteRegistrationRequest.fromPartial({ cognitoSub, username, email }),
    getMetadata(jwt),
  )
}
