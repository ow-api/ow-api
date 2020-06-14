curl -X POST "$UPLOAD_URL" -F "file_i386=@/build/i386/owapi_${VERSION}_i386.deb" \
  -F "file_amd64=@/build/amd64/owapi_${VERSION}_amd64.deb" \
  -F "file_armv7=@/build/armv7/owapi_${VERSION}_armv7.deb"