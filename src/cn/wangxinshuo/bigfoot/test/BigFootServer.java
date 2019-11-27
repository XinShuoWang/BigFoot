package cn.wangxinshuo.bigfoot.test;

import cn.wangxinshuo.bigfoot.server.conf.ServerConfiguration;
import cn.wangxinshuo.bigfoot.server.listen.ServerListenSocket;

import java.io.File;
import java.io.FileInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.util.Properties;

public class BigFootServer {
    public static void main(String[] args) {
        // 创建配置文件流
        try (InputStream inputStream = new FileInputStream(new File(args[0]))) {
            Properties properties = new Properties();
            // 加载配置文件流
            properties.load(inputStream);
            ServerConfiguration configuration = new ServerConfiguration();
            // set configuration
            configuration.setDestinationAddress(properties.getProperty("REMOTE_ADDRESS"));
            configuration.setDestinationPort(Integer.valueOf(properties.getProperty("REMOTE_PORT")));
            configuration.setListenPort(Integer.valueOf(properties.getProperty("LOCAL_PORT")));
            // SSL
            System.setProperty("javax.net.ssl.keyStore",
                    properties.getProperty("keyStore"));
            System.setProperty("javax.net.ssl.keyStorePassword",
                    properties.getProperty("keyStorePassword"));
            System.setProperty("javax.net.ssl.trustStore",
                    properties.getProperty("trustStore"));
            System.setProperty("javax.net.ssl.trustStorePassword",
                    properties.getProperty("trustStorePassword"));
            // load
            ServerListenSocket serverListenSocket = new ServerListenSocket(configuration);
            serverListenSocket.listen();
        } catch (IOException e) {
            e.printStackTrace();
        }
    }
}
