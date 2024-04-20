from fastapi import FastAPI, Depends, HTTPException, status
from fastapi.security import OAuth2AuthorizationCodeBearer
from starlette.responses import RedirectResponse
import httpx
import uvicorn
from dotenv import load_dotenv
import os
import base64

load_dotenv()
# Constants for the API endpoint and the local server
BASE_URL = "http://localhost:8000"
CLIENT_ID = os.getenv("SPOTIFY_CLIENT_ID")
CLIENT_SECRET = os.getenv("SPOTIFY_CLIENT_SECRET")



def get_access_token(client_id, client_secret):
    url = "https://accounts.spotify.com/api/token"
    headers = {
        "Authorization": "Basic " + base64.b64encode(f"{client_id}:{client_secret}".encode()).decode(),
        "Content-Type": "application/x-www-form-urlencoded"
    }
    data = {
        "grant_type": "client_credentials"
    }
    with httpx.Client() as client:
        response = client.post(url, headers=headers, data=data)
        if response.status_code != 200:
            print("Failed to fetch token:", response.status_code, response.text)
            return None
        response_data = response.json()
        return response_data.get("access_token")

def search_playlists(token, query):
    url = f"{BASE_URL}/search-playlists"
    headers = {"Authorization": f"Bearer {token}"}
    params = {"query": query}
    with httpx.Client() as client:
        response = client.get(url, headers=headers, params=params)
        print("HTTP Status:", response.status_code)  # Output status code for debugging
        print("Response Body:", response.text)       # Output full response body for debugging
        return response.json()

def get_playlist_contents(token, playlist_id):
    url = f"{BASE_URL}/playlist-contents/{playlist_id}"
    headers = {"Authorization": f"Bearer {token}"}
    with httpx.Client() as client:
        response = client.get(url, headers=headers)
        return response.json()

# Main execution block to perform the API calls
if __name__ == "__main__":
    access_token = get_access_token(CLIENT_ID, CLIENT_SECRET)
    if not access_token:
        print("Failed to retrieve access token")
        exit()
    
    playlist_name = "Classical Essentials"  # Change to your desired playlist name to search

    # Step 1: Search for playlists by name
    search_results = search_playlists(access_token, playlist_name)
    print("Search Results:", search_results)

    # Step 2: Check if playlists were found and get the contents of the first playlist
    if 'playlists' in search_results and search_results['playlists']['items']:
        first_playlist_id = search_results['playlists']['items'][0]['id']
        playlist_contents = get_playlist_contents(access_token, first_playlist_id)
        print("Playlist Contents:", playlist_contents)
    else:
        print("No playlists found or there was an error in the search.")


def load_dotenv(
    dotenv_path: Optional[StrPath] = None,
    stream: Optional[IO[str]] = None,
    verbose: bool = False,
    override: bool = False,
    interpolate: bool = True,
    encoding: Optional[str] = "utf-8",
) -> bool:
    """Parse a .env file and then load all the variables found as environment variables.

    Parameters:
        dotenv_path: Absolute or relative path to .env file.
        stream: Text stream (such as `io.StringIO`) with .env content, used if
            `dotenv_path` is `None`.
        verbose: Whether to output a warning the .env file is missing.
        override: Whether to override the system environment variables with the variables
            from the `.env` file.
        encoding: Encoding to be used to read the file.
    Returns:
        Bool: True if at least one environment variable is set else False

    If both `dotenv_path` and `stream` are `None`, `find_dotenv()` is used to find the
    .env file.
    """
    if dotenv_path is None and stream is None:
        dotenv_path = find_dotenv()

    dotenv = DotEnv(
        dotenv_path=dotenv_path,
        stream=stream,
        verbose=verbose,
        interpolate=interpolate,
        override=override,
        encoding=encoding,
    )
    return dotenv.set_as_environment_variables()
