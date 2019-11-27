package cn.wangxinshuo.bigfoot.client.forward;


import cn.wangxinshuo.bigfoot.client.conf.ClientConfiguration;
import cn.wangxinshuo.bigfoot.util.encryption.decrypt.Decrypt;

import java.io.*;
import java.net.Socket;

/*
* 将返回的数据写回
* */
public class ClientReverseSocketReceive extends Thread {
    private Socket client, destination;
    private ClientConfiguration configuration;

    public ClientReverseSocketReceive(Socket client, Socket destination, ClientConfiguration configuration) {
        this.client = client;
        this.destination = destination;
        this.configuration = configuration;
    }

    @Override
    public void run() {
        // 从目的端口拿到返回数据写回源socket
        try {
            byte []data = new byte[1024];
            // 将数据返回给请求端口
            OutputStream outputStream = client.getOutputStream();
            // 创建从目的地读取数据的流
            InputStream inputStream = destination.getInputStream();
            int size = 0;
            // 循环等待数据发送
            while ((size = inputStream.read(data, 0, data.length)) != -1) {
                // 将服务器发来的数据进行解密，然后返回
                byte []decryptResult = Decrypt.decrypt(data, size, configuration);
                // 写回给请求端
                outputStream.write(decryptResult, 0, decryptResult.length);
                outputStream.flush();
                System.out.println("BigFootServer ----> BigFootClient: " + String.valueOf(size) + " 字节");
            }
        }catch (IOException ignored){

        }finally {
            try {
                this.destination.close();
            } catch (IOException ignored) {

            }
        }
    }
}
