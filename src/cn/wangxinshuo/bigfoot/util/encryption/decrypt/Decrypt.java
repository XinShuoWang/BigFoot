package cn.wangxinshuo.bigfoot.util.encryption.decrypt;

import cn.wangxinshuo.bigfoot.conf.Configuration;
import cn.wangxinshuo.bigfoot.util.encryption.operation.Byte;

public class Decrypt {
    public static byte[] decrypt(byte[] data, int len, Configuration configuration){
        byte[] bytes = new byte[len];
        System.arraycopy(data,0,bytes,0,len);
        return Byte.byteDecrypt(bytes);
    }
}
