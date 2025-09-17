
# Test R2 upload
import requests
url = "https://lessnotestorage.r2.cloudflarestorage.com/notes/note-26.mp3?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=2%2F20250510%2F%2Fs3%2Faws4_request&X-Amz-Date=20250510T171939Z&X-Amz-Expires=900&X-Amz-SignedHeaders=host&x-id=PutObject&X-Amz-Signature=48439a1829832add5b939653bcbbfdf5914bb7e1c221e10e6e6df73459d0cdb4"
# https://lessnotestorage.375ad218a5792d29ba7b6cf710df4bc8.r2.cloudflarestorage.com/notes/images_1746897691472.zip?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=8c81eb12a1635df6d6fc204f2b3b279f%2F20250510%2F%2Fs3%2Faws4_request&X-Amz-Date=20250510T172134Z&X-Amz-Expires=900&X-Amz-SignedHeaders=host&x-id=PutObject&X-Amz-Signature=eb141dfb5d447c8a040f8d194329043b695cb4e564848557473e324b33a4d96d
resp = requests.put(url, files={'file': ('mytext.txt', 'HEY BRO')})
print(resp.content, resp.status_code)
