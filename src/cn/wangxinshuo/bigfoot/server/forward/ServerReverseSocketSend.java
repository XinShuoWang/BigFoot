package cn.wangxinshuo.bigfoot.server.forward;

import cn.wangxinshuo.bigfoot.util.encryption.decrypt.Decrypt;
import cn.wangxinshuo.bigfoot.server.conf.ServerConfiguration;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.Socket;
import java.util.Date;

/*
 * 把请求读进来，再转发
 * */
public class ServerReverseSocketSend extends Thread {
    private Socket client, destination;
    private ServerConfiguration configuration;

    public ServerReverseSocketSend(Socket client, Socket destination, ServerConfiguration configuration) {
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
                System.out.println(new Date().toString()
                        + "\tBigFootClient ---> BigFootServer ----> Squid: "
                        + String.valueOf(size) + " 字节");
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
