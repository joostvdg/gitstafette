resource "aws_route53_zone" "gistafette" {
  name = "gitstafette.joostvdg.net"

  tags = {
    Environment = "gitstafette"
  }
}

resource "aws_route53_record" "gistafette-ns" {
  zone_id = aws_route53_zone.gistafette.zone_id
  name    = "gitstafette.joostvdg.net"
  type    = "NS"
  ttl     = "30"
  records = aws_route53_zone.gistafette.name_servers
}

resource "aws_route53_record" "events" {
  zone_id = aws_route53_zone.gistafette.zone_id
  name    = "events.gitstafette.joostvdg.net"
  type    = "A"
  ttl     = 300
  records = [aws_eip.lb.public_ip]
}