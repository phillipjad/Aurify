import axios, { AxiosRequestConfig } from 'axios';
import { useOutletContext } from 'react-router-dom';

function Login() {

    const setUser: React.Dispatch<React.SetStateAction<{}>> = useOutletContext();

    const getToken = async() => {
        const config: AxiosRequestConfig = {
          method: "POST",
          url: "https://accounts.spotify.com/api/token",
          headers: {
            "Content-Type": "application/x-www-form-urlencoded"
          },
          data: {
            "grant_type": "client_credentials",
            "client_id": "35e3fc02755e4519b6e1b50d2c3b073e",
            "client_secret": "2fdacd747560444a9da15bb61fd8f16d"
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
            setUser({token});
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