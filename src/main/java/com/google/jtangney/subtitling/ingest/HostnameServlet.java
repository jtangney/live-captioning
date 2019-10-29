package com.google.jtangney.subtitling.ingest;

import javax.servlet.ServletException;
import javax.servlet.annotation.WebServlet;
import javax.servlet.http.HttpServlet;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import java.io.IOException;
import java.io.PrintWriter;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.util.logging.Logger;

@WebServlet("hostname")
public class HostnameServlet extends HttpServlet {

  public static String METADATA_URL = "http://metadata.google.internal/computeMetadata/v1/";
  public static String HOSTNAME_URL = METADATA_URL + "instance/hostname";

  private static Logger logger = Logger.getLogger(HostnameServlet.class.getName());
  private static final HttpClient httpClient = HttpClient.newBuilder()
      .version(HttpClient.Version.HTTP_2).build();

  protected void doGet(HttpServletRequest req, HttpServletResponse resp)
      throws ServletException, IOException {
    HttpRequest request = HttpRequest.newBuilder().GET()
        .uri(URI.create(HOSTNAME_URL))
        .header("Metadata-Flavor", "Google")
        .build();
    HttpResponse<String> response = null;
    try {
      response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
    }
    catch (InterruptedException e) {
      throw new ServletException(e);
    }
    String body = response.body();
    logger.info(body);

//    URL url = new URL(HOSTNAME_URL);
//    logger.info(HOSTNAME_URL);
//    HttpURLConnection con = (HttpURLConnection) url.openConnection();
//    con.addRequestProperty("Metadata-Flavor", "Google");
//    con.setRequestMethod("GET");
//
//    BufferedReader in = new BufferedReader(new InputStreamReader(con.getInputStream()));
//    StringBuilder sb = new StringBuilder();
//    String inputLine;
//    while ((inputLine = in.readLine()) != null)
//      sb.append(inputLine);
//    in.close();
//    logger.info(sb.toString());
//
    PrintWriter out = resp.getWriter();
    out.print(body);
    out.flush();
  }
}