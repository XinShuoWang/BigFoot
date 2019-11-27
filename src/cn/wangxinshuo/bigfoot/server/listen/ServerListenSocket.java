package cn.wangxinshuo.bigfoot.server.listen;

import cn.wangxinshuo.bigfoot.server.conf.ServerConfiguration;
import cn.wangxinshuo.bigfoot.server.forward.ServerReverseSocketReceive;
import cn.wangxinshuo.bigfoot.server.forward.ServerReverseSocketSend;
import cn.wangxinshuo.bigfoot.util.socket.ssl.SSLSocketBigFoot;

import javax.net.ssl.SSLServerSocket;
import javax.net.ssl.SSLSocket;
import java.io.IOException;
import java.net.ServerSocket;
import java.net.Socket;

public class ServerListenSocket {
    private ServerConfiguration configuration;
    private SSLServerSocket serverSocket;

    public ServerListenSocket(ServerConfiguration configuration) throws IOException {
        this.configuration = configuration;
        // init server socket
        serverSocket = SSLSocketBigFoot.getServerSocket(configuration.getListenPort());
    }

    public void listen() {
        while (true) {
            // 防止出错之后不继续
            try {
                // 未连通前线程阻塞，连通后开启一个socket通道线程后继续监听端口
                SSLSocket clientSocket = (SSLSocket) this.serverSocket.accept();
                Socket serverSocket = new Socket(configuration.getDestinationAddress(),
                        configuration.getDestinationPort());
                // 先开读取进程
                new ServerReverseSocketReceive(clientSocket, serverSocket, configuration).start();
                // 再开转发进程
                new ServerReverseSocketSend(clientSocket, serverSocket, configuration).start();
            }catch (IOException ignore){

            }
        }
    }
}
