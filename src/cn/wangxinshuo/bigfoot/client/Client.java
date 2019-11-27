package cn.wangxinshuo.bigfoot.client;

import cn.wangxinshuo.bigfoot.client.conf.ClientConfiguration;
import cn.wangxinshuo.bigfoot.client.listen.ClientListenSocket;
import cn.wangxinshuo.bigfoot.client.pac.PAC;

import java.io.File;
import java.io.FileInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.util.Properties;

public class Client {
    public static void main(String[] args) {
        // try打开配置文件
        try (InputStream inputStream = new FileInputStream(new File(args[0]))) {
            Properties properties = new Properties();
            // 加载配置文件
            properties.load(inputStream);
            ClientConfiguration configuration = new ClientConfiguration();
            /* 依据配置文件初始化配置类  */
            // 读入客户端相关配置文件
            configuration.setDestinationAddress(properties.getProperty("REMOTE_ADDRESS"));
            configuration.setDestinationPort(Integer.valueOf(properties.getProperty("REMOTE_PORT")));
            configuration.setListenPort(Integer.valueOf(properties.getProperty("LOCAL_PORT")));
            configuration.setHashMethod(properties.getProperty("HASH_METHOD"));
            configuration.setPassword(properties.getProperty("PASSWORD"));
            // 读入PAC相关配置文件
            configuration.setPacPort(Integer.valueOf(properties.getProperty("PAC_PORT")));
            // 启动PAC程序
            new PAC(configuration).start();
            // 启动客户端程序
            ClientListenSocket clientListenSocket = new ClientListenSocket(configuration);
            clientListenSocket.listen();
        } catch (IOException e) {
            e.printStackTrace();
        }
    }
}
