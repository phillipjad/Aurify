import React from 'react'
import ReactDOM from 'react-dom/client'
import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import App from './components/home/App.tsx'
import 'bootstrap/dist/css/bootstrap.css'
import './index.css'
import Login from './components/login/Login.tsx';
import Splash from './components/splash/Splash.tsx';
import Playlists from './components/user-bound/Playlists.tsx';
import About from './components/user-bound/About.tsx';
import Cover from './components/user-bound/Cover.tsx';

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
      },
      {
        path: "playlists",
        element: <Playlists/>
      },
      {
        path: "about",
        element: <About/>
      },
      {
        path: "cover",
        element: <Cover/>
      },
      {
        path: "*",
        element: <Login/>
      }
    ]
  }
])

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <RouterProvider router={router}/>
  </React.StrictMode>,
)
