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
SCOPE = "user-library-read"  # Example scope
AUTH_URL = "https://accounts.spotify.com/authorize"
TOKEN_URL = "https://accounts.spotify.com/api/token"

# Setting up OAuth2 scheme for FastAPI to use when getting the user info
oauth2_scheme = OAuth2AuthorizationCodeBearer(
    authorizationUrl=f"{AUTH_URL}?client_id={SPOTIFY_CLIENT_ID}&response_type=code&redirect_uri={REDIRECT_URI}&scope={SCOPE}",
    tokenUrl=TOKEN_URL
)

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
        access_token = token_response_json.get("access")
