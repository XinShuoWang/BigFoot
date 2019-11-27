package cn.wangxinshuo.bigfoot.util.socket.ssl;

import javax.net.ssl.*;
import java.io.IOException;


public class SSLSocketBigFoot {

    public static SSLServerSocket getServerSocket(Integer port) throws IOException {
        SSLServerSocketFactory socketFactory = (SSLServerSocketFactory) SSLServerSocketFactory.getDefault();
        return (SSLServerSocket) socketFactory.createServerSocket(port);
    }

    public static SSLSocket getClientSocket(String ip, Integer port) throws IOException {
        SSLSocketFactory socketFactory = (SSLSocketFactory) SSLSocketFactory.getDefault();
        return (SSLSocket) socketFactory.createSocket(ip, port);
    }
}
