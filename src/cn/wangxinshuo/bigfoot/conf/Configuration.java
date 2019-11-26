package cn.wangxinshuo.bigfoot.conf;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.Calendar;
import java.util.GregorianCalendar;

public class Configuration {
    private Integer listenPort;
    private String destinationAddress;
    private Integer destinationPort;
    private String hashMethod;
    private String password;

    private static String md5(String src) {
        try {
            // 加密对象，指定加密方式
            MessageDigest md5 = MessageDigest.getInstance("md5");
            // 准备要加密的数据
            byte[] b = src.getBytes();
            // 加密
            byte[] digest = md5.digest(b);
            // 十六进制的字符
            char[] chars = new char[]{'0', '1', '2', '3', '4', '5',
                    '6', '7', '8', '9', 'A', 'B', 'C', 'D', 'E', 'F'};
            StringBuilder sb = new StringBuilder();
            // 处理成十六进制的字符串(通常)
            for (byte bb : digest) {
                sb.append(chars[(bb >> 4) & 15]);
                sb.append(chars[bb & 15]);
            }
            // 打印加密后的字符串
            return sb.toString();
        } catch (NoSuchAlgorithmException e) {
            e.printStackTrace();
        }
        return null;
    }

    private static String byte2Hex(byte[] bytes) {
        StringBuilder stringBuffer = new StringBuilder();
        String temp = null;
        for (byte aByte : bytes) {
            temp = Integer.toHexString(aByte & 0xFF);
            if (temp.length() == 1) {
                //1得到一位的进行补0操作
                stringBuffer.append("0");
            }
            stringBuffer.append(temp);
        }
        return stringBuffer.toString();
    }

    private static String sha(String src, int bits) {
        MessageDigest messageDigest;
        String encodeStr = "";
        try {
            switch (bits){
                case 256:
                    messageDigest = MessageDigest.getInstance("SHA-256");
                case 512:
                    messageDigest = MessageDigest.getInstance("SHA-512");
                default:
                    messageDigest = MessageDigest.getInstance("SHA-256");
            }
            messageDigest.update(src.getBytes(StandardCharsets.UTF_8));
            encodeStr = byte2Hex(messageDigest.digest());
        } catch (NoSuchAlgorithmException e) {
            e.printStackTrace();
        }
        return encodeStr;
    }

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
                result = sha(strengthen_password, 256);
            case "SHA-512":
                result = sha(strengthen_password, 512);
            default:
                result = md5(strengthen_password);
        }
        assert result != null;
        return result.charAt(0);
    }

    public void setPassword(String password) {
        System.out.println("系统文件设置密码：" + password);
        this.password = password;
    }
}
