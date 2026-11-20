import { Component, type ErrorInfo, type ReactNode } from 'react'
import { Button, EmptyState } from '@aurorix/webui'

type Props = { children: ReactNode }
type State = { error: Error | null }

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('页面渲染失败', error, info.componentStack)
  }

  render() {
    const { error } = this.state
    if (!error) return this.props.children
    return (
      <div className="flex flex-col gap-4">
        <EmptyState title="这个页面出错了" description={error.message || '未知错误'} />
        <div className="flex gap-3">
          <Button onClick={() => this.setState({ error: null })}>重试</Button>
          <Button variant="secondary" onClick={() => window.location.reload()}>
            刷新控制台
          </Button>
        </div>
        <pre className="code-block max-h-64 overflow-auto">{error.stack || error.message}</pre>
      </div>
    )
  }
}
