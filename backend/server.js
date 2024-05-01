const express = require('express');
const cors = require('cors');
const axios = require('axios');
const app = express();

app.use(express.json());
app.use(cors());

app.get('/', (req, res) => {
    res.end('Hello World!');
})

app.get('/playlists', async (req, res) => {
    res.setHeader('Content-Type', 'application/json');
    res.setHeader('Access-Control-Allow-Origin', '*');
    let token = req.get('token');

    console.log(token);
    let playlistReqConfig = {
        method: "GET",
        url: "https://api.spotify.com/v1/me/playlists",
        headers: {
            "Authorization": "Bearer " + token
        }
    };
    let response = await axios(playlistReqConfig);
    console.log(response.data.items);
    res.send(response.data.items);
})

async function generate_image(audioFeatures, genres) {
    let audioString = `${audioFeatures[0]}, ${audioFeatures[1]}, ${audioFeatures[2]}, ${audioFeatures[3]}, ${audioFeatures[4]}, ${audioFeatures[5]}, ${audioFeatures[6]}, ${audioFeatures[7]}, and ${audioFeatures[8]}. All of these are averages for the album. All are measured from 0 to 1 except for loudness and tempo which are measured in decibals and beats per minute respectively. Most values are straightforward to interpet, but liveness represents how likely it is that the music was performed live and valence represents how positive a song is.`;
    let genreString = `${genres[0][0]}, ${genres[1][0]}, ${genres[2][0]}, ${genres[3][0]}, ${genres[4][0]}, ${genres[5][0]}, ${genres[6][0]}, ${genres[7][0]}, ${genres[8][0]}, and ${genres[9][0]}`;
    let prompt = `Generate an abstract, ethereal, and almost psychedelic album cover. The cover should be purely abstract with just colors and random geometry throughout. There should be absolutely no characters, no words, no typography, no graphs, no structure, and no text. The idea is to generate an abstract maelstrom of colors, shapes, lines, shadows, and reflections. This is purely for use as a cover that represents the following audio features: ${audioString} with the top ten genres: ${genreString}.`;
    let imageConfig = {
        method: "POST",
        url: "https://api.openai.com/v1/images/generations",
        headers: {
            "Content-Type": "application/json",
            "Authorization": "Bearer " + process.env.OPENAI_API_KEY
        },
        data: {
            "model": "dall-e-3",
            prompt,
            "style": "natural",
            "user": "Aurify"
        }
    };
    let url = await axios(imageConfig);
    return url.data.data[0].url
}

app.get('/playlist/image', async (req, res) => {
    res.setHeader('Content-Type', 'application/json');
    res.setHeader('Access-Control-Allow-Origin', '*');
    let token = req.get('token');
    let endpoint = req.get('endpoint');
    console.log(token);

    let genres = {};
    let acousticness = 0; //0-1
    let danceability = 0; //0-1
    let energy = 0; //0-1
    let instrumentalness = 0; //0-1
    let liveness = 0; //Represents live performance 0-1
    let loudness = 0; //in db
    let speechiness = 0; //0-1
    let tempo = 0;
    let valence = 0; //0-1
    
    let playlistReqConfig = {
        method: "GET",
        url: endpoint,
        headers: {
            "Authorization": "Bearer " + token
        }
    };
    let response = await axios(playlistReqConfig);
    tracks = response.data.items.map(track => track.track);
    tracks = tracks.slice(0, 50);

    let audioFeaturesReqConfig = {
        method: "GET",
        url: "https://api.spotify.com/v1/audio-features?ids=" + tracks.map(track => track.id).join(","),
        headers: {
            "Authorization": "Bearer " + token
        }
    }

    let artistReqConfig = {
        method: "GET",
        url: "https://api.spotify.com/v1/artists?ids=" + tracks.map(track => track.artists[0].id).join(","),
        headers: {
            "Authorization": "Bearer " + token
        }
    }

    let response2 = await Promise.all([axios(audioFeaturesReqConfig), axios(artistReqConfig)]);
    let [{ data: {audio_features: audioFeaturesData} }, { data: {artists: artistData} }] = response2;

    tracks = tracks.map((track, index) => {
        track.audioFeatures = audioFeaturesData[index];
        track.artists[0].genres = artistData[index].genres || [];
        return track
    });



    tracks.forEach((track) => {
        if (!track.audioFeatures) {
            return;
        }
        if (track.artists[0].genres){
            track.artists[0].genres.forEach(genre => {
                genres[genre] = (genres[genre] || 0) + 1
            })
        }
        acousticness += track.audioFeatures.acousticness;
        danceability += track.audioFeatures.danceability;
        energy += track.audioFeatures.energy;
        instrumentalness += track.audioFeatures.instrumentalness;
        liveness += track.audioFeatures.liveness;
        loudness += track.audioFeatures.loudness;
        speechiness += track.audioFeatures.speechiness;
        tempo += track.audioFeatures.tempo;
        valence += track.audioFeatures.valence;
    })

    let audioFeatures = {
        acousticness: acousticness / tracks.length,
        danceability: danceability / tracks.length,
        energy: energy / tracks.length,
        instrumentalness: instrumentalness / tracks.length,
        liveness: liveness / tracks.length,
        loudness: loudness / tracks.length,
        speechiness: speechiness / tracks.length,
        tempo: tempo / tracks.length,
        valence: valence / tracks.length
    }

    console.log(audioFeatures);
    genres = Object.entries(genres).sort((a, b) => b[1] - a[1]);
    genres = genres.slice(0, 10);
    console.log(genres);
    let imageURL = await generate_image(audioFeatures, genres);
    console.log(imageURL);
    res.send(imageURL);
});

app.listen(3000);