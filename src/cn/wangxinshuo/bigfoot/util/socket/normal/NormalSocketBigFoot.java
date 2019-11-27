package cn.wangxinshuo.bigfoot.util.socket.normal;

import java.io.IOException;
import java.net.ServerSocket;
import java.net.Socket;

public class NormalSocketBigFoot {
    public static ServerSocket getServerSocket(Integer port) throws IOException {
        return new ServerSocket(port);
    }

    public static Socket getClientSocket(String ip, Integer port) throws IOException {
        return new Socket(ip, port);
    }
}
