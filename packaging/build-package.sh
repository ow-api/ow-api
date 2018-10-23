fpm -s dir -t deb -p build/$ARCH/owapi_$VERSION.deb \
    -n ow-api -v $VERSION \
    --deb-priority optional --force \
    --deb-compression bzip2 \
    --description "Overwatch API Server" \
    -m "Tyler Stuyfzand <admin@meow.tf>" --vendor "Meow.tf" \
    --before-install packaging/scripts/preinst.deb \
    --after-install packaging/scripts/postinst.deb \
    -a $ARCH build/$ARCH/owapi=/usr/bin/owapi \
    packaging/owapi.service=/lib/systemd/system/owapi.service
