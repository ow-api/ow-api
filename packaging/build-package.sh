fpm -s dir -t deb -p /build/$ARCH/owapi_${VERSION}_${ARCH}.deb \
    -n ow-api -v $VERSION -a $ARCH \
    --deb-priority optional --force \
    --deb-compression gz \
    --description "Overwatch API Server" \
    -m "Tyler Stuyfzand <admin@meow.tf>" --vendor "Meow.tf" \
    --before-install packaging/scripts/preinst.deb \
    --after-install packaging/scripts/postinst.deb \
    -a $ARCH /build/$ARCH/owapi_${ARCH}=/usr/bin/owapi \
    packaging/owapi.service=/lib/systemd/system/owapi.service
