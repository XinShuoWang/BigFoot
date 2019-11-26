package cn.wangxinshuo.bigfoot.server;

import cn.wangxinshuo.bigfoot.conf.Configuration;
import cn.wangxinshuo.bigfoot.encryption.decrypt.Decrypt;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.Socket;

/*
 * 把请求读进来，再转发
 * */
class ReverseSocketBigFootSend extends Thread {
    private Socket client, destination;
    private Configuration configuration;

    ReverseSocketBigFootSend(Socket client, Socket destination, Configuration configuration) {
        this.client = client;
        this.destination = destination;
        this.configuration = configuration;
    }

    @Override
    public void run() {
        // 拿到待转发的数据,发送数据给目的端口
        try {
            byte[] data = new byte[1024];
            OutputStream outputStream = destination.getOutputStream();
            InputStream inputStream = client.getInputStream();
            int size = 0;
            while ((size = inputStream.read(data, 0, data.length)) != -1) {
                byte[] decryptResult = Decrypt.decrypt(data, size, configuration);
                outputStream.write(decryptResult, 0, decryptResult.length);
                outputStream.flush();
                System.out.println("此次Send读取字节数：" + String.valueOf(size));
            }
        } catch (IOException ignored) {

        } finally {
            try {
                this.client.close();
            } catch (IOException ignored) {

            }
        }
    }
}
