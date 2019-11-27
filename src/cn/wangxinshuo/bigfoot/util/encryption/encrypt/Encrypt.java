package cn.wangxinshuo.bigfoot.util.encryption.encrypt;

import cn.wangxinshuo.bigfoot.conf.Configuration;
import cn.wangxinshuo.bigfoot.util.encryption.operation.Byte;

public class Encrypt {
    public static byte[] encrypt(byte[] data, int len, Configuration configuration){
        byte[] bytes = new byte[len];
        System.arraycopy(data,0,bytes,0,len);
        return bytes;
    }
}
