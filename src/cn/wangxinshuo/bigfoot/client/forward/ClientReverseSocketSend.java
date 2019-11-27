package cn.wangxinshuo.bigfoot.client.forward;

import cn.wangxinshuo.bigfoot.client.conf.ClientConfiguration;
import cn.wangxinshuo.bigfoot.util.encryption.encrypt.Encrypt;

import javax.net.ssl.SSLSocket;
import java.io.*;
import java.net.Socket;
import java.util.Date;

/*
 * 把请求读进来，再转发
 * */
public class ClientReverseSocketSend extends Thread {
    private Socket client;
    private SSLSocket destination;
    private ClientConfiguration configuration;

    public ClientReverseSocketSend(Socket client, SSLSocket destination, ClientConfiguration configuration) {
        this.client = client;
        this.destination = destination;
        this.configuration = configuration;
    }

    @Override
    public void run() {
        // 拿到待转发的数据,发送数据给目的端口
        try {
            byte[] data = new byte[1024];
            // 向目的端口内部写入数据的流
            OutputStream outputStream = destination.getOutputStream();
            // 从客户端口接受流
            InputStream inputStream = client.getInputStream();
            int size = 0;
            // 边读边加密之后转发
            while ((size = inputStream.read(data, 0, data.length)) != -1) {
                // 加密数据
                byte[] encryptResult = Encrypt.encrypt(data, size, configuration);
                // 写入数据
                outputStream.write(encryptResult, 0, encryptResult.length);
                outputStream.flush();
                System.out.println(new Date().toString()
                        + "\tProxy ---> BigFootClient ----> BigFootServer: "
                        + String.valueOf(size) + " 字节");
            }
        } catch (IOException ignored) {

        } finally {
            try {
                // 关闭客户端socket
                this.client.close();
            } catch (IOException ignored) {

            }
        }
    }
}
