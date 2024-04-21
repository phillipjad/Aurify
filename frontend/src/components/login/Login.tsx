import axios, { AxiosRequestConfig } from 'axios';
import { useOutletContext, useNavigate, redirect, NavigateFunction, useSearchParams } from 'react-router-dom';
import { nanoid } from 'nanoid';
import { useEffect, useState } from 'react';

function Login() {
  const [searchParams] = useSearchParams();
  let code = (searchParams.get("code"));
  const setToken: React.Dispatch<React.SetStateAction<{}>> = useOutletContext();
  const navigate = useNavigate();
  const client_id = "35e3fc02755e4519b6e1b50d2c3b073e"
  const client_secret = "2fdacd747560444a9da15bb61fd8f16d"
  const state = nanoid(16);
  const scope = "user-read-private user-read-email";
  const redirect_uri = "http://localhost:5173/";
  const url = `https://accounts.spotify.com/authorize?response_type=code&client_id=${client_id}&scope=${scope}&redirect_uri=${redirect_uri}&state=${state}`;

  useEffect(() => {
    if (!code) {
      window.location.href = url;
    }
  }, [code])


  const getToken = async () => {
    const config: AxiosRequestConfig = {
      method: "POST",
      url: "https://accounts.spotify.com/api/token",
      headers: {
        "Authorization": "Basic " + btoa(client_id+ ":" + client_secret),
        "Content-Type": "application/x-www-form-urlencoded"
      },
      data: {
        "grant_type": "authorization_code",
        code,
        redirect_uri
      }
    }

    const token = await axios(config).then((res)=>{
      return res.data.access_token;
    }).catch((err)=>{
      console.error(err);
      return undefined;
    });

    if (token){
        console.log(token);
        navigate("/home");
        setToken(token);
    }
  }

  return (
    <button
      onClick={getToken}
      className="bg-green-700 p-3 border-2 border-green-600 rounded rounded-4 text-xl"
    >
      Login to Spotify
    </button>
  );
}

export default Login;