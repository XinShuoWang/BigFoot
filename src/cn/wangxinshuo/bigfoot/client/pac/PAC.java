package cn.wangxinshuo.bigfoot.client.pac;

import cn.wangxinshuo.bigfoot.client.conf.ClientConfiguration;

public class PAC extends Thread {
    private ClientConfiguration configuration;

    public PAC(ClientConfiguration configuration){
        this.configuration = configuration;
    }

    @Override
    public void run() {

    }
}
