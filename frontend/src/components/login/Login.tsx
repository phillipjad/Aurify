import axios, { AxiosRequestConfig } from 'axios';
import { useOutletContext, useNavigate, useSearchParams } from 'react-router-dom';
import { nanoid } from 'nanoid';
import { useEffect } from 'react';
import SpotifyLogo from '../../assets/spotify.svg';
import { outletContext } from '../../utils/types';

/**
 * Renders a login button that, when clicked, initiates the Spotify OAuth flow.
 *
 * @return {JSX.Element} A button element that triggers the login flow.
 */
function Login() {
  const [searchParams] = useSearchParams();
  let code = (searchParams.get("code"));
  const {token, setToken} = useOutletContext<outletContext>();
  const navigate = useNavigate();
  const client_id = "35e3fc02755e4519b6e1b50d2c3b073e"
  const client_secret = "2fdacd747560444a9da15bb61fd8f16d"
  const state = nanoid(16);
  const scope = "user-read-private user-read-email";
  const redirect_uri = "http://localhost:5173/";
  const url = `https://accounts.spotify.com/authorize?response_type=code&client_id=${client_id}&scope=${scope}&redirect_uri=${redirect_uri}&state=${state}`;

  
  useEffect(() => {
    let needToken: boolean = false;
    if (code && !token){
      needToken = true;
    }
    console.log(`code: ${code}, requestLogin: ${needToken}, token: ${token}`);
    
    if (!token && needToken) {
      getToken(token);
    }

  }, [code]
  )

    /**
   * Function to retrieve a token from Spotify API.
   *
   * @return {Promise<void>} A promise that resolves with the access token or undefined.
   */
  const getToken = async (token: string) => {
    if (token){
      navigate("/home");
      return;
    }
    if (!code) {
      window.location.href = url;
    }
    const config: AxiosRequestConfig = {
      method: "POST",
      url: "https://accounts.spotify.com/api/token",
      headers: {
        "Authorization": "Basic " + btoa(client_id + ":" + client_secret),
        "Content-Type": "application/x-www-form-urlencoded"
      },
      data: {
        "grant_type": "authorization_code",
        code,
        redirect_uri
      }
    }

    const resToken = await axios(config).then((res) => {
      return res.data.access_token;
    }).catch((err) => {
      console.error(err);
      return undefined;
    });

    if (resToken) {
      console.log(resToken);
      navigate("/home");
      setToken(resToken);
    }
    return;
  }

  return (
    <button
      style={{userSelect: 'none'}}
      onClick={() => {
        getToken(token);
      }}
      className="bg-green-700 hover:bg-green-600 p-3 border-2 border-green-600 rounded rounded-4 text-xl"
    >
      Login to Spotify <img className='ms-2 w-8 inline' src={SpotifyLogo} alt="Spotify logo" />
    </button>
  );
}

export default Login;