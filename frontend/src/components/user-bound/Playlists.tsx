import { useEffect, useState } from "react";
import axios, { AxiosRequestConfig } from "axios";
import { Playlist, outletContext } from "../../utils/types";
import { useOutletContext, useNavigate } from "react-router-dom";

function Playlists() {
    const navigate = useNavigate();
    const [playlists, setPlaylist] = useState<Playlist[] | null>(null);
    const { token, user } = useOutletContext<outletContext>();

    function getImage(playlist: Playlist, token: string) {
        console.log(playlist);
        let trackReqConfig: AxiosRequestConfig = {
            method: "GET",
            url: `http://localhost:3000/playlist/image`,
            headers: {
                token,
                endpoint: playlist.tracks.href
            }
        }
        axios(trackReqConfig).then((res) => {
            let imageURL = res.data;
            console.log(imageURL);
            navigate("/cover", { state: { imageURL } });
        })
    }

    useEffect(() => {
        if (!token) {
            return;
        }
        let playlistReqConfig: AxiosRequestConfig = {
            method: "GET",
            url: "http://localhost:3000/playlists",
            headers: {
                token
            }
        }
        axios(playlistReqConfig).then((res) => {
            let playlistData = res.data;
            console.log(playlistData);
            setPlaylist(playlistData);
        });
    }, []);

    return (
        <div
            className="align-items-center justify-center w-100 inline-flex flex-col"
        >
            {user && <h1 className="text-3xl font-bold">Select a Playlist to Generate a Cover!</h1>}
            <div className="d-grid w-50 sm:max-lg:w-100 overflow-y-auto gap-2" style={{
                display: "grid",
                gridTemplateColumns: "repeat(auto-fill, minmax(300px, 1fr))",
                height: "60dvh"
            }}>
                {playlists && user && playlists.length > 0 && (
                    playlists.filter(playlist => playlist.collaborative === false && playlist.owner.display_name === user.display_name).map(playlist => (
                            <button key={playlist.id} className="align-middle m-2 border-gray-700 bg-no-repeat bg-center bg-cover rounded-xl border-5"
                                style={{
                                    backgroundColor: 'rgba(0, 0, 0, 0.5)',
                                    backgroundImage: playlist.images && playlist.images.length > 0 ? `url(${playlist.images[0].url})` : '',
                                    height: "300px"
                                }}
                                onClick={() => getImage(playlist, token)}>
                                <div className="d-flex flex-wrap font-bold justify-content-center align-content-center bg-gray-600 h-20 bg-opacity-90 text-white">{playlist.name}</div>
                            </button>
                    ))
                )}
            </div>
        </div>
    );
}

export default Playlists;
