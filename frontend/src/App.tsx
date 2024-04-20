import { Outlet } from 'react-router-dom'
import Splash from './components/splash/Splash'

function App() {
  return (
    <>
      <Splash />
      <div className="current-route">
        <Outlet />
      </div>
    </>
  )
}

export default App
