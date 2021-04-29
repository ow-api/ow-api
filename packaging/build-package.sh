for arch in $(echo $ARCH | sed "s/,/ /g"); do
  fpm -s dir -t deb -p /build/owapi_${VERSION}_${arch}.deb \
      -n ow-api -v $VERSION -a $arch \
      --deb-priority optional --force \
      --deb-compression gz \
      --description "Overwatch API Server" \
      -m "Tyler Stuyfzand <admin@meow.tf>" --vendor "Meow.tf" \
      --before-install packaging/scripts/preinst.deb \
      --after-install packaging/scripts/postinst.deb \
      /build/owapi_${arch}=/usr/bin/owapi \
      packaging/owapi.service=/lib/systemd/system/owapi.service
done