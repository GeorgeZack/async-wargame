source ~/.config/opencode/api-key.env
OPENCODE_SESSION_ID="$(uuidgen)"

curl -i https://opencode.ai/zen/go/v1/chat/completions \
  -H "Authorization: Bearer $OPENCODE_API_KEY" \
  -H "Content-Type: application/json" \
  -H "x-opencode-session: $OPENCODE_SESSION_ID" \
  -A "my-opencode-test/1.0" \
  -d '{
    "model": "glm-5.2",
    "messages": [{"role": "user", "content": "Say OK"}]
  }'
