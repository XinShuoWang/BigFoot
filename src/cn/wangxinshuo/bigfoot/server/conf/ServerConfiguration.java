package cn.wangxinshuo.bigfoot.server.conf;

import cn.wangxinshuo.bigfoot.conf.Configuration;
import cn.wangxinshuo.bigfoot.util.hash.Hash;

import java.util.Calendar;
import java.util.GregorianCalendar;

public class ServerConfiguration implements Configuration {
    private Integer listenPort;
    private String destinationAddress;
    private Integer destinationPort;
    private String hashMethod;
    private String password;

    public Integer getListenPort() {
        return listenPort;
    }

    public void setListenPort(Integer listenPort) {
        System.out.println("本地监听端口：" + String.valueOf(listenPort));
        this.listenPort = listenPort;
    }

    public String getDestinationAddress() {
        return destinationAddress;
    }

    public void setDestinationAddress(String destinationAddress) {
        System.out.println("目的地址：" + destinationAddress);
        this.destinationAddress = destinationAddress;
    }

    public Integer getDestinationPort() {
        return destinationPort;
    }

    public void setDestinationPort(Integer destinationPort) {
        System.out.println("目的端口：" + String.valueOf(destinationPort));
        this.destinationPort = destinationPort;
    }

    public String getHashMethod() {
        return hashMethod;
    }

    public void setHashMethod(String hashMethod) {
        System.out.println("Hash算法：" + hashMethod);
        this.hashMethod = hashMethod;
    }

    public int getPassword() {
        String result;
        Calendar calendar = new GregorianCalendar();
        String strengthen_password = password + String.valueOf(calendar.get(Calendar.YEAR))
                + String.valueOf(calendar.get(Calendar.MONTH))
                + String.valueOf(calendar.get(Calendar.DAY_OF_MONTH))
                + String.valueOf(calendar.get(Calendar.HOUR_OF_DAY));
        switch (this.getHashMethod()) {
            case "SHA-256":
                result = Hash.sha(strengthen_password, 256);
            case "SHA-512":
                result = Hash.sha(strengthen_password, 512);
            default:
                result = Hash.md5(strengthen_password);
        }
        assert result != null;
        return result.charAt(0);
    }

    public void setPassword(String password) {
        System.out.println("系统文件设置密码：" + password);
        this.password = password;
    }
}
