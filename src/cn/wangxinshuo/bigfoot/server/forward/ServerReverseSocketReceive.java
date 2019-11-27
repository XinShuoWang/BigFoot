package cn.wangxinshuo.bigfoot.server.forward;

import cn.wangxinshuo.bigfoot.util.encryption.encrypt.Encrypt;
import cn.wangxinshuo.bigfoot.server.conf.ServerConfiguration;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.Socket;

/*
* 将返回的数据写回
* */
public class ServerReverseSocketReceive extends Thread {
    private Socket client, destination;
    private ServerConfiguration configuration;

    public ServerReverseSocketReceive(Socket client, Socket destination, ServerConfiguration configuration) {
        this.client = client;
        this.destination = destination;
        this.configuration = configuration;
    }

    @Override
    public void run() {
        // 从目的端口拿到返回数据写回源socket
        try {
            byte []data = new byte[1024];
            // 创建流
            OutputStream outputStream = client.getOutputStream();
            InputStream inputStream = destination.getInputStream();
            int size = 0;
            // 不间断读取
            while ((size = inputStream.read(data, 0, data.length)) != -1) {
                // 从squid得到的数据要加密之后才能放到网上传输
                byte []encryptResult = Encrypt.encrypt(data, size, configuration);
                outputStream.write(encryptResult, 0, encryptResult.length);
                outputStream.flush();
                System.out.println("Squid ----> BigFootServer: " + String.valueOf(size) + " 字节");
            }
        }catch (IOException ignored){

        }finally {
            try {
                // 关闭socket
                this.destination.close();
            } catch (IOException ignored) {

            }
        }
    }
}
