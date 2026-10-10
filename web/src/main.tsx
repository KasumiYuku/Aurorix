import { render } from 'solid-js/web'
import App from './app'
import { initTheme } from './theme'
import './styles/index.css'

initTheme()

render(() => <App />, document.getElementById('app')!)