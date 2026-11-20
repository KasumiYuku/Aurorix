import type { SceneScope } from './types'

export const SCENE_LABELS: Record<SceneScope, string> = {
  group: '群聊',
  c2c: '私聊',
  channel: '频道',
  dm: '频道私信',
}

export const SCENE_SHORT: Record<SceneScope, string> = {
  group: '群',
  c2c: '私聊',
  channel: '频道',
  dm: '私信',
}

export function sceneLabel(scene: SceneScope): string {
  return SCENE_LABELS[scene] ?? scene
}

export function sceneShort(scene: SceneScope): string {
  return SCENE_SHORT[scene] ?? scene
}

export function showsSenderName(scene: SceneScope): boolean {
  return scene === 'group' || scene === 'channel'
}
