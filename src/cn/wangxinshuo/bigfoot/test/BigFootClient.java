package cn.wangxinshuo.bigfoot.test;

import cn.wangxinshuo.bigfoot.client.ListenSocketBigFoot;
import cn.wangxinshuo.bigfoot.conf.Configuration;

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
            Configuration configuration = new Configuration();
            // 依据配置文件初始化配置类
            configuration.setDestinationAddress(properties.getProperty("REMOTE_ADDRESS"));
            configuration.setDestinationPort(Integer.valueOf(properties.getProperty("REMOTE_PORT")));
            configuration.setListenPort(Integer.valueOf(properties.getProperty("LOCAL_PORT")));
            configuration.setHashMethod(properties.getProperty("HASH_METHOD"));
            configuration.setPassword(properties.getProperty("PASSWORD"));
            // 启动客户端程序
            ListenSocketBigFoot listenSocketBigFoot = new ListenSocketBigFoot(configuration);
            listenSocketBigFoot.listen();
        } catch (IOException e) {
            e.printStackTrace();
        }
    }
}
