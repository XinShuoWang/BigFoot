package cn.wangxinshuo.bigfoot.util.socket.ssl;

import javax.net.ssl.SSLServerSocket;
import javax.net.ssl.SSLServerSocketFactory;
import javax.net.ssl.SSLSocket;
import javax.net.ssl.SSLSocketFactory;
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
