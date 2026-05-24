import { ENVOY_HOST, getMetadata } from './transport'
import {
  GrpcWebImpl,
  CashierServiceClientImpl,
  CheckRequest,
} from '../gen/cashier/cashier'

const client = new CashierServiceClientImpl(new GrpcWebImpl(ENVOY_HOST, {}))

export function checkAvailable(userId: string, jwt: string) {
  return client.CheckAvailable(CheckRequest.fromPartial({ userId }), getMetadata(jwt))
}
