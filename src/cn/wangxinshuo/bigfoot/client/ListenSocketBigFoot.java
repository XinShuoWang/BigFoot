package cn.wangxinshuo.bigfoot.client;

import cn.wangxinshuo.bigfoot.conf.Configuration;

import java.io.IOException;
import java.net.ServerSocket;
import java.net.Socket;

public class ListenSocketBigFoot {
    private Configuration configuration;
    private ServerSocket serverSocket;

    public ListenSocketBigFoot(Configuration configuration) throws IOException {
        this.configuration = configuration;
        // init server socket
        serverSocket = new ServerSocket(configuration.getListenPort());
    }

    public void listen() throws IOException {
        while (true) {
            // 未连通前线程阻塞，连通后开启一个socket通道线程后继续监听端口
            Socket clientSocket = this.serverSocket.accept();
            Socket serverSocket = new Socket(configuration.getDestinationAddress(),
                    configuration.getDestinationPort());
            System.out.println("创建Socket完毕！开启转发线程！");
            // 先准备好接收程序
            new ReverseSocketBigFootReceive(clientSocket, serverSocket, configuration).start();
            // 开启转发请求程序
            new ReverseSocketBigFootSend(clientSocket, serverSocket, configuration).start();
        }
    }
}
