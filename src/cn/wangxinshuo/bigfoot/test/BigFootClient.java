package cn.wangxinshuo.bigfoot.test;

import cn.wangxinshuo.bigfoot.client.Client;
import cn.wangxinshuo.bigfoot.client.conf.ClientConfiguration;
import cn.wangxinshuo.bigfoot.client.listen.ClientListenSocket;

import java.io.File;
import java.io.FileInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.util.Properties;

public class BigFootClient {
    public static void main(String[] args) {
        // try打开配置文件
        try (InputStream inputStream = new FileInputStream(new File(args[0]))) {
            Properties properties = new Properties();
            // 加载配置文件
            properties.load(inputStream);
            ClientConfiguration configuration = new ClientConfiguration();
            // 依据配置文件初始化配置类
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
            // 启动客户端程序
            ClientListenSocket clientListenSocket = new ClientListenSocket(configuration);
            clientListenSocket.listen();
        } catch (IOException e) {
            e.printStackTrace();
        }
    }
}
