package cn.wangxinshuo.bigfoot.server;

import cn.wangxinshuo.bigfoot.conf.Configuration;

import java.io.File;
import java.io.FileInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.util.Properties;

public class Server {
    public static void main(String[] args) {
        // 创建配置文件流
        try (InputStream inputStream = new FileInputStream(new File(args[0]))) {
            Properties properties = new Properties();
            // 加载配置文件流
            properties.load(inputStream);
            Configuration configuration = new Configuration();
            // set configuration
            configuration.setDestinationAddress(properties.getProperty("REMOTE_ADDRESS"));
            configuration.setDestinationPort(Integer.valueOf(properties.getProperty("REMOTE_PORT")));
            configuration.setListenPort(Integer.valueOf(properties.getProperty("LOCAL_PORT")));
            configuration.setHashMethod(properties.getProperty("HASH_METHOD"));
            configuration.setPassword(properties.getProperty("PASSWORD"));
            // load
            ListenSocketBigFoot listenSocketBigFoot = new ListenSocketBigFoot(configuration);
            listenSocketBigFoot.listen();
        } catch (IOException e) {
            e.printStackTrace();
        }
    }
}