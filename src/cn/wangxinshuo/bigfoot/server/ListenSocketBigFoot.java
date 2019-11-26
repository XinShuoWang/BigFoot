package cn.wangxinshuo.bigfoot.server;

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
            // 先开读取进程
            new ReverseSocketBigFootReceive(clientSocket, serverSocket, configuration).start();
            // 再开转发进程
            new ReverseSocketBigFootSend(clientSocket, serverSocket, configuration).start();
            System.out.println("转发线程启动完毕！");
        }
    }
}
