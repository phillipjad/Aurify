# Replace YOUR_CLIENT_ID and YOUR_CLIENT_SECRET with your actual Spotify credentials
echo -n "8827289762fe4a2eb143c61569dce83c:57907cce2fde4ea9a3fce819dbbc8881" | base64

# Use the output of the above command in the following cURL request
curl -X POST -H "Authorization: Basic ODgyNzI4OTc2MmZlNGEyZWIxNDNjNjE1NjlkY2U4M2M6NTc5MDdjY2UyZmRlNGVhOWEzZmNlODE5ZGJiYzg4ODE=" -d "grant_type=client_credentials" "https://accounts.spotify.com/api/token"
