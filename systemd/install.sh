cd gateway
GOOS=linux GOARCH=amd64 make release
cd ../api
GOOS=linux GOARCH=amd64 make release
cd ../analytics
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 make release
cd ../systemd
scp -i ~/.ssh/openwrt ../gateway/app root@104.168.96.115:/usr/bin/gateway
scp -i ~/.ssh/openwrt ../api/app root@104.168.96.115:/usr/bin/stopbot-api
scp -i ~/.ssh/openwrt ../analytics/app root@104.168.96.115:/usr/bin/stopbot-analytics
scp -i ~/.ssh/openwrt proxy.service root@103.252.119.26:/etc/systemd/system/
scp -i ~/.ssh/openwrt stopbot-analytics.service root@104.168.96.115:/etc/systemd/system/
scp -i ~/.ssh/openwrt stopbot-api.service root@104.168.96.115:/etc/systemd/system/
ssh -i ~/.ssh/openwrt root@104.168.96.115 'systemctl daemon-reload && systemctl start gateway'
ssh -i ~/.ssh/openwrt root@104.168.96.115 'systemctl daemon-reload && systemctl start stopbot-analytics'
ssh -i ~/.ssh/openwrt root@104.168.96.115 'systemctl daemon-reload && systemctl start stopbot-api'
