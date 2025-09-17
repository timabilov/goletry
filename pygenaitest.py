
import os
from google import genai
from google.genai import types
client = genai.Client(
    api_key=os.environ["GOOGLE_API_KEY"],
    http_options=types.HttpOptions(api_version='v1beta')
)


response = client.models.generate_content(
    model='gemini-2.5-flash-preview-04-17', contents='Say hello?',
)

# 'user-agent': 'google-genai-sdk/1.12.1 gl-python/3.9.22', 
# 'x-goog-api-client': 'google-genai-sdk/1.12.1 gl-python/3.9.22'}
print(response)