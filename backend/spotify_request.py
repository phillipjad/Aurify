from fastapi import FastAPI, Depends, HTTPException, status
from fastapi.security import OAuth2AuthorizationCodeBearer
from starlette.responses import RedirectResponse
import httpx
import uvicorn
from dotenv import load_dotenv
import os
import base64

load_dotenv()

app = FastAPI()

SPOTIFY_CLIENT_ID = os.getenv("SPOTIFY_CLIENT_ID")
SPOTIFY_CLIENT_SECRET = os.getenv("SPOTIFY_CLIENT_SECRET")
REDIRECT_URI = "http://localhost:8000/callback"
SCOPE = "user-library-read playlist-read-private"
AUTH_URL = "https://accounts.spotify.com/authorize"
TOKEN_URL = "https://accounts.spotify.com/api/token"

# Setting up OAuth2 scheme for FastAPI to use when getting the user info
oauth2_scheme = OAuth2AuthorizationCodeBearer(
    authorizationUrl=f"{AUTH_URL}?client_id={SPOTIFY_CLIENT_ID}&response_type=code&redirect_uri={REDIRECT_URI}&scope={SCOPE}",
    tokenUrl=TOKEN_URL
)

@app.get("/")
async def root():
    return {"message": "Hello World"}

@app.get("/login")
def login():
    return RedirectResponse(
        f"{AUTH_URL}?client_id={SPOTIFY_CLIENT_ID}&response_type=code&redirect_uri={REDIRECT_URI}&scope={SCOPE}"
    )

@app.get("/callback")
async def callback(code: str):
    token_data = {
        "grant_type": "authorization_code",
        "code": code,
        "redirect_uri": REDIRECT_URI,
    }
    token_headers = {
        "Authorization": "Basic " + base64.b64encode(f"{SPOTIFY_CLIENT_ID}:{SPOTIFY_CLIENT_SECRET}".encode()).decode()
    }
    
    async with httpx.AsyncClient() as client:
        token_response = await client.post(TOKEN_URL, data=token_data, headers=token_headers)
        if token_response.status_code != 200:
            raise HTTPException(status_code=token_response.status_code, detail="Failed to retrieve access token")
        token_response_json = token_response.json()
        access_token = token_response_json.get("access_token")
        return {"access_token": access_token}

@app.get("/search-playlists")
async def search_playlists(query: str, token: str = Depends(oauth2_scheme)):
    search_url = "https://api.spotify.com/v1/search"
    headers = {
        "Authorization": f"Bearer {token}"
    }
    params = {
        "q": query,
        "type": "playlist",
        "limit": 10  # Adjust the limit as needed
    }
    async with httpx.AsyncClient() as client:
        response = await client.get(search_url, headers=headers, params=params)
        if response.status_code != 200:
            raise HTTPException(status_code=response.status_code, detail="Failed to search playlists")
        return response.json()

@app.get("/playlist-contents/{playlist_id}")
async def playlist_contents(playlist_id: str, token: str = Depends(oauth2_scheme)):
    playlist_url = f"https://api.spotify.com/v1/playlists/{playlist_id}"
    headers = {
        "Authorization": f"Bearer {token}"
    }
    async with httpx.AsyncClient() as client:
        response = await client.get(playlist_url, headers=headers)
        if response.status_code != 200:
            raise HTTPException(status_code=response.status_code, detail="Failed to retrieve playlist contents")
        return response.json()

if __name__ == "__main__":
    uvicorn.run(app, host="localhost", port=8000)
