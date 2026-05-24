import { Outcome, PropType, Comparator, type Leg } from '../gen/registry/registry'

export const OUTCOME_LABELS: Partial<Record<Outcome, string>> = {
  [Outcome.HOME_WIN]:  'Home Win',
  [Outcome.HOME_LOSS]: 'Away Win',
  [Outcome.DRAW]:      'Draw',
}

export const CMP_LABELS: Partial<Record<Comparator, string>> = {
  [Comparator.GT]:  '>',
  [Comparator.GTE]: '>=',
  [Comparator.LT]:  '<',
  [Comparator.LTE]: '<=',
  [Comparator.EQ]:  '=',
}

export const PROP_LABELS: Partial<Record<PropType, string>> = {
  [PropType.HOMERUNS]:        'HRs',
  [PropType.STRIKEOUTS]:      'Ks',
  [PropType.HITS]:            'Hits',
  [PropType.RBIS]:            'RBIs',
  [PropType.WALKS]:           'BBs',
  [PropType.EARNED_RUNS]:     'ERs',
  [PropType.GOALS]:           'Goals',
  [PropType.ASSISTS]:         'Assists',
  [PropType.SAVES]:           'Saves',
  [PropType.YELLOW_CARDS]:    'Yellow Cards',
  [PropType.PASSING_YARDS]:   'Pass Yds',
  [PropType.RUSHING_YARDS]:   'Rush Yds',
  [PropType.RECEIVING_YARDS]: 'Rec Yds',
  [PropType.TOUCHDOWNS]:      'TDs',
  [PropType.INTERCEPTIONS]:   'INTs',
}

export function describeLeg(leg: Leg): string {
  if (leg.gameOutcome) {
    return `${leg.gameId} — ${OUTCOME_LABELS[leg.gameOutcome.outcome] ?? leg.gameOutcome.outcome}`
  }
  if (leg.playerProp) {
    const pp = leg.playerProp
    return `${pp.playerId} ${PROP_LABELS[pp.propType] ?? pp.propType} ${CMP_LABELS[pp.comparator] ?? pp.comparator} ${pp.threshold}`
  }
  return leg.gameId
}

export function fmtExpiry(unix: number): string {
  return new Date(unix * 1000).toLocaleString(undefined, {
    month: 'short', day: 'numeric', year: 'numeric',
    hour: 'numeric', minute: '2-digit',
  })
}
