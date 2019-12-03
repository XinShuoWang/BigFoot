# BigFoot
## 优点：
1. 跨平台：使用Java开发，原生跨平台。
2. 无依赖，可执行JAR包体积较小。
3. 支持自定义协议，用户可以根据封锁情况自定义协议进行加密，默认使用SSL加密。
4. 不易被检测：使用SSL加密，与合法流量融合。
5. 使用OSS存储PAC文件，云端更新，不用担心无法更新PAC文件。
6. 服务端可以用浏览器访问，增强抗封锁能力与隐蔽性。
7. 增加多用户细粒度管理能力。
8. 增加管理API统计用户流量使用情况。

## TODO:
1. IPv6支持。
2. 增加一个Web管理页面。

## 服务端用法
[下载地址](https://github.com/XinShuoWang/BigFoot/releases/download/2.0/BigFootServer.tar.gz)
Linux Shell
```shell script
apt update && apt install nginx squid openjdk-8-jre-headless -y
sed -i "1164i http_access allow all"  /etc/squid/squid.conf
wget <URL of BigFootServer.jar>
systemctl start squid.service
echo "Hello World!" > /var/www/html/index.html
vim server.properties
java -jar BigFootServer.jar server.properties > /dev/null &
```

## 客户端用法
[下载地址](https://github.com/XinShuoWang/BigFoot/releases/download/2.0/BigFootClient.zip)
1. 设置PAC文件（PAC文件设置默认在1080监听本地端口）, 
可用的一个在线PAC文件https://xinshuowang-pac.oss-cn-shanghai.aliyuncs.com/pac.txt，
注意如果自己生成PAC文件需要设置文件的HTTP响应头为：Content-Type：application/x-ns-proxy-autoconfig
2. 下载BigFoot客户端
3. 配置JRE（Java Runtime Environment）
4. 访问指定网站，点击证书，并保存；使用
``` .\keytool -import -alias certificatekey -file C:\Users\XinShuoWang\Documents\key\xxx.cer -keystore C:\Users\XinShuoWang\Documents\key\xxx.truststore```
命令来生成TrustStore文件，此文件可以保证SSL连接的安全性。
5. 执行程序```java -jar BigFootClient client.properties```