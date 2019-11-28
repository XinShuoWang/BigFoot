package cn.wangxinshuo.bigfoot.test;

import java.io.IOException;
import java.net.URL;

public class SSL {
    public static void main(String[] args) {
        try {
            new URL("https://helloworld.letsencrypt.org").openConnection().connect();
        } catch (IOException e) {
            e.printStackTrace();
        }
    }
}
