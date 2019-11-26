package cn.wangxinshuo.bigfoot.server;

import cn.wangxinshuo.bigfoot.conf.Configuration;
import cn.wangxinshuo.bigfoot.encryption.decrypt.Decrypt;
import cn.wangxinshuo.bigfoot.encryption.encrypt.Encrypt;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.Socket;

/*
* 将返回的数据写回
* */
class ReverseSocketBigFootReceive extends Thread {
    private Socket client, destination;
    private Configuration configuration;

    ReverseSocketBigFootReceive(Socket client, Socket destination, Configuration configuration) {
        this.client = client;
        this.destination = destination;
        this.configuration = configuration;
    }

    @Override
    public void run() {
        // 从目的端口拿到返回数据写回源socket
        try {
            System.out.println("Receive线程开始工作！");
            byte []data = new byte[1024];
            // 创建流
            OutputStream outputStream = client.getOutputStream();
            InputStream inputStream = destination.getInputStream();
            int size = 0;
            System.out.println("Receive线程的输入输出流构建完毕！等待数据！");
            // 不间断读取
            while ((size = inputStream.read(data, 0, data.length)) != -1) {
                // 从squid得到的数据要加密之后才能放到网上传输
                byte []encryptResult = Encrypt.encrypt(data, size, configuration);
                outputStream.write(encryptResult, 0, encryptResult.length);
                outputStream.flush();
                System.out.println("此次Receive读取字节数：" + String.valueOf(size));
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
