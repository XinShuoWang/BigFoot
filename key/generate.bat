keytool -genkey -alias sslclient -keystore sslclientkeys
keytool -export -alias sslclient -keystore sslclientkeys -file sslclient.cer
keytool -genkey -alias sslserver -keystore sslserverkeys
keytool -export -alias sslserver -keystore sslserverkeys -file sslserver.cer
keytool -import -alias sslclient -keystore sslservertrust -file sslclient.cer
keytool -import -alias sslserver -keystore sslclienttrust -file sslserver.cer
keytool -list -keystore sslclienttrust