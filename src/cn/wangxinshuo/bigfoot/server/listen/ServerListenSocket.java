package cn.wangxinshuo.bigfoot.server.listen;

import cn.wangxinshuo.bigfoot.server.conf.ServerConfiguration;
import cn.wangxinshuo.bigfoot.server.forward.ServerReverseSocketReceive;
import cn.wangxinshuo.bigfoot.server.forward.ServerReverseSocketSend;

import java.io.IOException;
import java.net.ServerSocket;
import java.net.Socket;

public class ServerListenSocket {
    private ServerConfiguration configuration;
    private ServerSocket serverSocket;

    public ServerListenSocket(ServerConfiguration configuration) throws IOException {
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
            // 先开读取进程
            new ServerReverseSocketReceive(clientSocket, serverSocket, configuration).start();
            // 再开转发进程
            new ServerReverseSocketSend(clientSocket, serverSocket, configuration).start();
        }
    }
}
