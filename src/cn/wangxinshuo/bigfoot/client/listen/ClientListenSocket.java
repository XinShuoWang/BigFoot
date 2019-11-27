package cn.wangxinshuo.bigfoot.client.listen;

import cn.wangxinshuo.bigfoot.client.conf.ClientConfiguration;
import cn.wangxinshuo.bigfoot.client.forward.ClientReverseSocketReceive;
import cn.wangxinshuo.bigfoot.client.forward.ClientReverseSocketSend;
import cn.wangxinshuo.bigfoot.util.socket.ssl.SSLSocketBigFoot;

import javax.net.ssl.SSLServerSocket;
import javax.net.ssl.SSLSocket;
import java.io.IOException;
import java.net.ServerSocket;
import java.net.Socket;

public class ClientListenSocket {
    private ClientConfiguration configuration;
    private ServerSocket serverSocket;

    public ClientListenSocket(ClientConfiguration configuration) throws IOException {
        this.configuration = configuration;
        // init server socket
        serverSocket = new ServerSocket(configuration.getListenPort());
    }

    public void listen() throws IOException {
        while (true) {
            // 未连通前线程阻塞，连通后开启一个socket通道线程后继续监听端口
            Socket clientSocket = this.serverSocket.accept();
            SSLSocket serverSocket = SSLSocketBigFoot
                    .getClientSocket(configuration.getDestinationAddress(),
                            configuration.getDestinationPort());
            // 先准备好接收程序
            new ClientReverseSocketReceive(clientSocket, serverSocket, configuration).start();
            // 开启转发请求程序
            new ClientReverseSocketSend(clientSocket, serverSocket, configuration).start();
        }
    }
}
