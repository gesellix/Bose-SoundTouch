/etc/init.d/aftertouch stop
rm -rf /etc/init.d/aftertouch
update-rc.d -f aftertouch remove
rm -rf /opt/aftertouch