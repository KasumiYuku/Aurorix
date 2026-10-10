import { createSignal, onCleanup } from 'solid-js'

function parse(): string[] {
  return location.hash.replace(/^#\/?/, '').split('/').filter(Boolean)
}

export function createRoute() {
  const [parts, setParts] = createSignal<string[]>(parse())
  const onChange = () => {
    const apply = () => setParts(parse())
    if (document.startViewTransition) document.startViewTransition(apply)
    else apply()
  }
  window.addEventListener('hashchange', onChange)
  onCleanup(() => window.removeEventListener('hashchange', onChange))
  return parts
}

export function navigate(to: string) {
  if (location.hash === '#/' + to) return
  location.hash = '/' + to
}
