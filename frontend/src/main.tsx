import React from 'react'
import ReactDOM from 'react-dom/client'
import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import App from './components/home/App.tsx'
import 'bootstrap/dist/css/bootstrap.css'
import './index.css'
import Login from './components/login/Login.tsx';
import Splash from './components/splash/Splash.tsx';

const router = createBrowserRouter([
  {
    path: "/",
    element: <App/>,
    children: [
      {
        path: "",
        element: <Login/>
      },
      {
        path: "home",
        element: <Splash/>
      }
    ]
  }
])

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <RouterProvider router={router}/>
  </React.StrictMode>,
)
