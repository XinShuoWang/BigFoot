package cn.wangxinshuo.bigfoot.client.conf;

import cn.wangxinshuo.bigfoot.conf.Configuration;

public class ClientConfiguration implements Configuration {
    private Integer listenPort;
    private String destinationAddress;
    private Integer destinationPort;
    // SSL
    private String keyStore;
    private String keyStorePassword;
    private String trustStore;
    private String trustStorePassword;
    //
    private byte[] camouflage;

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

    public String getKeyStore() {
        return keyStore;
    }

    public void setKeyStore(String keyStore) {
        this.keyStore = keyStore;
    }

    public String getKeyStorePassword() {
        return keyStorePassword;
    }

    public void setKeyStorePassword(String keyStorePassword) {
        this.keyStorePassword = keyStorePassword;
    }

    public String getTrustStore() {
        return trustStore;
    }

    public void setTrustStore(String trustStore) {
        this.trustStore = trustStore;
    }

    public String getTrustStorePassword() {
        return trustStorePassword;
    }

    public void setTrustStorePassword(String trustStorePassword) {
        this.trustStorePassword = trustStorePassword;
    }

    public byte[] getCamouflage() {
        return camouflage;
    }

    public void setCamouflage(String camouflage) {
        System.out.println("有效屏蔽头：" + camouflage);
        this.camouflage = camouflage.getBytes();
    }
}
