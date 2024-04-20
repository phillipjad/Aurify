SPOTIFY_CLIENT_ID=your_spotify_client_id
SPOTIFY_CLIENT_SECRET=your_spotify_client_secret


from fastapi import FastAPI, Depends
from starlette.responses import RedirectResponse
import httpx
from dotenv import load_dotenv
import os

load_dotenv()

app = FastAPI()

SPOTIFY_CLIENT_ID = os.getenv("SPOTIFY_CLIENT_ID")
SPOTIFY_CLIENT_SECRET = os.getenv("SPOTIFY_CLIENT_SECRET")
REDIRECT_URI = "http://localhost:8000/callback"
SCOPE = "user-library-read"  # Example scope
AUTH_URL = "https://accounts.spotify.com/authorize"

@app.get("/login")
def login():
    return RedirectResponse(
        f"{AUTH_URL}?client_id={SPOTIFY_CLIENT_ID}&response_type=code&redirect_uri={REDIRECT_URI}&scope={SCOPE}"
    )

@app.get("/callback")
async def callback(code: str):
    token_url = "https://accounts.spotify.com/api/token"
    token_data = {
        "grant_type": "authorization_code",
        "code": code,
        "redirect_uri": REDIRECT_URI,
    }
    token_headers = {
        "Authorization": f"Basic {httpx.BasicAuth(SPOTIFY_CLIENT_ID, SPOTIFY_CLIENT_SECRET).encode()}"
    }
    
    async with httpx.AsyncClient() as client:
        token_response = await client.post(token_url, data=token_data, headers=token_headers)
        token_response_json = token_response.json()
        access_token = token_response_json["access_token"]
        return {"access_token": access_token}

@app.get("/userinfo")
async def userinfo(token: str = Depends(oauth2_scheme)):
    headers = {
        "Authorization": f"Bearer {token}"
    }
    user_info_endpoint = "https://api.spotify.com/v1/me"
    async with httpx.AsyncClient() as client:
        user_info_response = await client.get(user_info_endpoint, headers=headers)
        return user_info_response.json()
