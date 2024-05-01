import { useEffect, useState } from 'react'
import axios, { AxiosRequestConfig } from 'axios';
import { Container } from 'react-bootstrap'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { SpotifyUser, outletContext } from '../../utils/types';

function App() {
  let location = useLocation();
  const navigate = useNavigate();
  const [token, setToken] = useState("");
  const [user, setUser] = useState<SpotifyUser | null>(null);

  function goHome(){
    navigate("/home");
  }

  useEffect(() => {
    console.log(token);
    if (token) {
      let userConfig: AxiosRequestConfig = {
        method: "GET",
        url: "https://api.spotify.com/v1/me",
        headers: {
          "Authorization": "Bearer " + token
        }
      };
      axios(userConfig).then((res) => {
        let user = res.data as SpotifyUser;
        setUser(user);
      });
    }
  }, [token])

  useEffect(() => {
    console.log("user", user);
  }, [user])

  return (
    <Container
      className="d-flex flex-column align-items-center justify-content-center"
      fluid
    >
      <div
        className='w-100 d-flex flex-row flex-wrap justify-content-center align-items-center align-items-center'
      >
        {location.pathname !== "/home" && location.pathname !== "/" && <div className="me-auto">
          <svg
            onClick={goHome}
            className='w-12 h-12 hover:text-slate-500'
          xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" d="M15.75 19.5 8.25 12l7.5-7.5" />
          </svg>
        </div>}
        <div className={location.pathname !== "/home" && location.pathname !== "/"  ? "me-auto" : "mx-auto"}>
          <h1
            style={{ userSelect: 'none' }}
            className='text-8xl text-green-500 mt-4 text-center kaushan-script-regular'>
            Aurify
          </h1>
        </div>
      </div>
      {user && <h3
        className='mt-5 select-none'
      >Greetings, {user.display_name}
      </h3>}
      <main
        id='current-route'
        className="d-flex mt-5 container-fluid justify-content-center"
      >
        <Outlet context={
          { token, setToken, user } satisfies outletContext
        } />
      </main>
    </Container>
  )
}

export default App
